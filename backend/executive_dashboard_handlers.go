package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExecutiveDashboardResponse represents executive dashboard data
type ExecutiveDashboardResponse struct {
	RiskTrend          []RiskTrendPoint        `json:"risk_trend"`
	TopRemediated      []TopRemediatedIssue    `json:"top_remediated"`
	CompliancePosture  CompliancePosture       `json:"compliance_posture"`
	TotalRisks         int                     `json:"total_risks"`
	CriticalRisks      int                     `json:"critical_risks"`
	Resolved24h        int                     `json:"resolved_24h"`
	Resolved7d         int                     `json:"resolved_7d"`
}

type RiskTrendPoint struct {
	Date          string `json:"date"`
	Total         int    `json:"total"`
	Critical      int    `json:"critical"`
	High          int    `json:"high"`
	Medium        int    `json:"medium"`
	Low           int    `json:"low"`
}

type TopRemediatedIssue struct {
	RiskType      string `json:"risk_type"`
	Count         int    `json:"count"`
	AvgResolutionTime string `json:"avg_resolution_time"`
}

type CompliancePosture struct {
	CriticalResolved24h    float64 `json:"critical_resolved_24h"`    // Percentage
	AllRisksResolved7d     float64 `json:"all_risks_resolved_7d"`     // Percentage
	ComplianceScore        float64 `json:"compliance_score"`          // Overall score
}

// GetExecutiveDashboard returns executive dashboard data
func GetExecutiveDashboard(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// Get risk trend (last 30 days)
		trend, err := getRiskTrend(ctx, pool, 30)
		if err != nil {
			// Return empty trend instead of error
			trend = []RiskTrendPoint{}
		}

		// Get top remediated issues
		topRemediated, err := getTopRemediated(ctx, pool)
		if err != nil {
			// Return empty list instead of error
			topRemediated = []TopRemediatedIssue{}
		}

		// Get compliance posture
		compliance, err := getCompliancePosture(ctx, pool)
		if err != nil {
			// Return default compliance posture instead of error
			compliance = CompliancePosture{
				CriticalResolved24h: 0,
				AllRisksResolved7d:  0,
				ComplianceScore:     0,
			}
		}

		// Get current risk counts
		var totalRisks, criticalRisks int
		err = pool.QueryRow(ctx, `
			SELECT COUNT(*), COUNT(*) FILTER (WHERE max_severity = 'critical')
			FROM risk_scores
		`).Scan(&totalRisks, &criticalRisks)
		if err != nil {
			// Table might not exist or be empty, use defaults
			totalRisks = 0
			criticalRisks = 0
		}

		// Get resolved counts
		var resolved24h, resolved7d int
		err = pool.QueryRow(ctx, `
			SELECT 
				COALESCE(COUNT(*) FILTER (WHERE resolved_at >= NOW() - INTERVAL '24 hours' AND risk_type LIKE '%critical%'), 0),
				COALESCE(COUNT(*) FILTER (WHERE resolved_at >= NOW() - INTERVAL '7 days'), 0)
			FROM remediation_history
		`).Scan(&resolved24h, &resolved7d)
		if err != nil {
			// Table might not exist or be empty, use defaults
			resolved24h = 0
			resolved7d = 0
		}

		c.JSON(http.StatusOK, ExecutiveDashboardResponse{
			RiskTrend:         trend,
			TopRemediated:     topRemediated,
			CompliancePosture: compliance,
			TotalRisks:        totalRisks,
			CriticalRisks:     criticalRisks,
			Resolved24h:       resolved24h,
			Resolved7d:        resolved7d,
		})
	}
}

func getRiskTrend(ctx context.Context, pool *pgxpool.Pool, days int) ([]RiskTrendPoint, error) {
	// 1) Prefer risk_trends table if it has data
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT date, total_identities, critical_count, high_count, medium_count, low_count
		FROM risk_trends
		WHERE date >= CURRENT_DATE - INTERVAL '%d days'
		ORDER BY date ASC
		LIMIT %d
	`, days, days))
	if err == nil {
		defer rows.Close()
		var trend []RiskTrendPoint
		for rows.Next() {
			var point RiskTrendPoint
			var date time.Time
			if err := rows.Scan(&date, &point.Total, &point.Critical, &point.High, &point.Medium, &point.Low); err != nil {
				continue
			}
			point.Date = date.Format("2006-01-02")
			trend = append(trend, point)
		}
		if len(trend) > 0 {
			return trend, nil
		}
	}

	// 2) Fallback: build trend from risk_scores (group by date of computed_at) so chart shows something
	type dayCounts struct {
		date   time.Time
		total  int
		crit   int
		high   int
		med    int
		low    int
	}
	scoreRows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT
			date(computed_at) AS d,
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE max_severity = 'critical') AS critical_count,
			COUNT(*) FILTER (WHERE max_severity = 'high') AS high_count,
			COUNT(*) FILTER (WHERE max_severity = 'medium') AS medium_count,
			COUNT(*) FILTER (WHERE max_severity = 'low') AS low_count
		FROM risk_scores
		WHERE computed_at >= CURRENT_DATE - INTERVAL '%d days'
		GROUP BY date(computed_at)
		ORDER BY d ASC
	`, days))
	if err != nil {
		return []RiskTrendPoint{}, nil
	}
	defer scoreRows.Close()

	byDate := make(map[string]RiskTrendPoint)
	for scoreRows.Next() {
		var c dayCounts
		if err := scoreRows.Scan(&c.date, &c.total, &c.crit, &c.high, &c.med, &c.low); err != nil {
			continue
		}
		key := c.date.Format("2006-01-02")
		byDate[key] = RiskTrendPoint{
			Date:     key,
			Total:    c.total,
			Critical: c.crit,
			High:     c.high,
			Medium:   c.med,
			Low:      c.low,
		}
	}

	// Fill last N days so chart always has 30 points (zeros for missing days)
	var trend []RiskTrendPoint
	for i := days - 1; i >= 0; i-- {
		d := time.Now().UTC().AddDate(0, 0, -i)
		key := d.Format("2006-01-02")
		if p, ok := byDate[key]; ok {
			trend = append(trend, p)
		} else {
			trend = append(trend, RiskTrendPoint{Date: key, Total: 0, Critical: 0, High: 0, Medium: 0, Low: 0})
		}
	}
	return trend, nil
}

func getTopRemediated(ctx context.Context, pool *pgxpool.Pool) ([]TopRemediatedIssue, error) {
	rows, err := pool.Query(ctx, `
		SELECT 
			risk_type,
			COUNT(*) as count,
			AVG(time_to_resolve) as avg_resolution_time
		FROM remediation_history
		WHERE status = 'executed'
		GROUP BY risk_type
		ORDER BY count DESC
		LIMIT 10
	`)
	if err != nil {
		// Return empty list if table doesn't exist or query fails
		return []TopRemediatedIssue{}, nil
	}
	defer rows.Close()

	var top []TopRemediatedIssue
	for rows.Next() {
		var issue TopRemediatedIssue
		var avgTime *time.Duration
		if err := rows.Scan(&issue.RiskType, &issue.Count, &avgTime); err != nil {
			continue
		}
		if avgTime != nil {
			issue.AvgResolutionTime = avgTime.String()
		} else {
			issue.AvgResolutionTime = "N/A"
		}
		top = append(top, issue)
	}

	return top, nil
}

func getCompliancePosture(ctx context.Context, pool *pgxpool.Pool) (CompliancePosture, error) {
	var posture CompliancePosture

	// Calculate critical risks resolved within 24h
	// Use a simpler query that doesn't require joins if tables are empty
	err := pool.QueryRow(ctx, `
		SELECT 
			COALESCE(
				(COUNT(*) FILTER (WHERE risk_type LIKE '%critical%' AND resolved_at >= NOW() - INTERVAL '24 hours')::FLOAT / 
				 NULLIF(COUNT(*) FILTER (WHERE risk_type LIKE '%critical%'), 0)::FLOAT * 100),
				0
			) as critical_24h,
			COALESCE(
				(COUNT(*) FILTER (WHERE resolved_at >= NOW() - INTERVAL '7 days')::FLOAT / 
				 NULLIF(COUNT(*), 0)::FLOAT * 100),
				0
			) as all_7d
		FROM remediation_history
	`).Scan(&posture.CriticalResolved24h, &posture.AllRisksResolved7d)
	if err != nil {
		// Use fallback calculation if table doesn't exist or is empty
		posture.CriticalResolved24h = 0
		posture.AllRisksResolved7d = 0
	}

	// Calculate overall compliance score (weighted average)
	posture.ComplianceScore = (posture.CriticalResolved24h*0.6 + posture.AllRisksResolved7d*0.4)

	return posture, nil
}
