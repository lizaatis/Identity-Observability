package main

// Mock dataset structures matching test_dataset.json

type MockUser struct {
	SourceSystem string `json:"source_system"`
	SourceID     string `json:"source_id"`
	Email        string `json:"email"`
	DisplayName  string `json:"display_name"`
	EmployeeID   string `json:"employee_id"`
	Status       string `json:"status"`
}

type MockGroup struct {
	SourceSystem string `json:"source_system"`
	SourceID     string `json:"source_id"`
	Name         string `json:"name"`
}

type MockRole struct {
	SourceSystem   string `json:"source_system"`
	SourceID       string `json:"source_id"`
	Name           string `json:"name"`
	PrivilegeLevel string `json:"privilege_level"`
}

type MockPermission struct {
	SourceSystem string `json:"source_system"`
	SourceID     string `json:"source_id"`
	Name         string `json:"name"`
	ResourceType string `json:"resource_type"`
}

type MockIdentityGroup struct {
	SourceSystem   string `json:"source_system"`
	SourceUserID   string `json:"source_user_id"`
	SourceGroupID  string `json:"source_group_id"`
}

type MockIdentityRole struct {
	SourceSystem  string `json:"source_system"`
	SourceUserID  string `json:"source_user_id"`
	SourceRoleID  string `json:"source_role_id"`
}

type MockGroupRole struct {
	SourceSystem  string `json:"source_system"`
	SourceGroupID string `json:"source_group_id"`
	SourceRoleID  string `json:"source_role_id"`
}

type MockRolePermission struct {
	SourceSystem       string `json:"source_system"`
	SourceRoleID       string `json:"source_role_id"`
	SourcePermissionID string `json:"source_permission_id"`
}

type MockDataset struct {
	Users          []MockUser          `json:"users"`
	Groups         []MockGroup         `json:"groups"`
	Roles          []MockRole          `json:"roles"`
	Permissions    []MockPermission    `json:"permissions"`
	IdentityGroup  []MockIdentityGroup `json:"identity_group"`
	IdentityRole   []MockIdentityRole  `json:"identity_role"`
	GroupRole      []MockGroupRole     `json:"group_role"`
	RolePermission []MockRolePermission `json:"role_permission"`
}
