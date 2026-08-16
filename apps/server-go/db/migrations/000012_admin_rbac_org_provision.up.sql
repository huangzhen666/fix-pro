-- Provision built-in tenant admin role for organizations created after PLAN-009.
CREATE OR REPLACE FUNCTION provision_admin_rbac_for_org()
RETURNS TRIGGER AS $$
DECLARE
    role_id BIGINT;
BEGIN
    INSERT INTO admin_role (org_id, role_code, name, description, status, is_builtin)
    VALUES (NEW.id, 'tenant_admin', '租户管理员', '拥有本租户全部后台权限', 'ACTIVE', TRUE)
    ON CONFLICT (org_id, role_code) DO UPDATE SET name = EXCLUDED.name
    RETURNING id INTO role_id;
    INSERT INTO admin_role_permission (org_id, role_id, permission_id)
    SELECT NEW.id, role_id, p.id FROM admin_permission p WHERE p.status = 'ACTIVE'
    ON CONFLICT DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_organization_provision_admin_rbac ON organization;
CREATE TRIGGER trg_organization_provision_admin_rbac
AFTER INSERT ON organization FOR EACH ROW EXECUTE FUNCTION provision_admin_rbac_for_org();
