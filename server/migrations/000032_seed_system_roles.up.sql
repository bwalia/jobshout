-- Backfill the four system roles for every existing organization and grant the
-- admin role to each org's admin users. Registration seeds these for new orgs
-- (auth_service.seedOwnerRole); before this migration no code path did, so
-- every org's RBAC tables were empty and permission-guarded surfaces (the chat
-- agent's tool guard) denied even the org owner.
--
-- The permission arrays mirror model.SystemRolePermissions (internal/model/rbac.go).

INSERT INTO roles (org_id, name, permissions, is_system)
SELECT o.id, r.name, r.permissions, true
FROM organizations o
CROSS JOIN (VALUES
    ('admin', ARRAY[
        'agents:create','agents:read','agents:update','agents:delete','agents:execute',
        'tasks:create','tasks:read','tasks:update','tasks:delete',
        'projects:create','projects:read','projects:update','projects:delete',
        'workflows:create','workflows:read','workflows:update','workflows:delete','workflows:execute',
        'budgets:read','budgets:manage','policies:read','policies:manage',
        'analytics:read','cost:read','audit:read',
        'users:manage','roles:manage','sso:manage','org:manage']),
    ('operator', ARRAY[
        'agents:create','agents:read','agents:update','agents:execute',
        'tasks:create','tasks:read','tasks:update',
        'projects:create','projects:read','projects:update',
        'workflows:create','workflows:read','workflows:update','workflows:execute',
        'analytics:read']),
    ('viewer', ARRAY[
        'agents:read','tasks:read','projects:read','workflows:read',
        'analytics:read']),
    ('finance', ARRAY[
        'budgets:read','budgets:manage',
        'analytics:read','cost:read','audit:read',
        'policies:read'])
) AS r(name, permissions)
ON CONFLICT (org_id, name) DO NOTHING;

-- Grant admin to each org's admin users (users.role is the legacy column the
-- REST layer trusts; RBAC now agrees with it).
INSERT INTO user_roles (user_id, role_id, org_id)
SELECT u.id, r.id, u.org_id
FROM users u
JOIN roles r ON r.org_id = u.org_id AND r.name = 'admin' AND r.is_system
WHERE u.role = 'admin' AND u.org_id IS NOT NULL
ON CONFLICT (user_id, role_id, org_id) DO NOTHING;
