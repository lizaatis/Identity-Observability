interface RiskBadgeProps {
  score: number;
  severity: string;
}

export default function RiskBadge({ score, severity }: RiskBadgeProps) {
  const getColor = () => {
    if (score >= 70) return 'bg-red-100 text-red-800 border-red-300';
    if (score >= 40) return 'bg-orange-100 text-orange-800 border-orange-300';
    if (score >= 20) return 'bg-yellow-100 text-yellow-800 border-yellow-300';
    return 'bg-green-100 text-green-800 border-green-300';
  };

  return (
    <div className={`px-4 py-2 rounded-lg border-2 ${getColor()}`}>
      <div className="text-2xl font-bold">{score}</div>
      <div className="text-xs uppercase tracking-wide">{severity}</div>
    </div>
  );
}
