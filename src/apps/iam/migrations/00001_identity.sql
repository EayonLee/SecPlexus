-- +goose Up
CREATE SCHEMA IF NOT EXISTS iam;
CREATE TABLE iam.users (
 id uuid PRIMARY KEY,
 username text NOT NULL CHECK (username ~ '^[A-Za-z][A-Za-z0-9_.-]{2,63}$'),
 username_normalized text NOT NULL UNIQUE CHECK(username_normalized=lower(username)),
 display_name text NOT NULL CHECK(char_length(display_name) BETWEEN 1 AND 100),
 email text CHECK(char_length(email)<=320),
 status text NOT NULL CHECK(status IN ('active','disabled')),
 is_super_admin boolean NOT NULL DEFAULT false,
 must_change_password boolean NOT NULL DEFAULT true,
 auth_version bigint NOT NULL DEFAULT 1 CHECK(auth_version>0),
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE iam.password_credentials (
 user_id uuid PRIMARY KEY REFERENCES iam.users(id),password_hash text NOT NULL,
 changed_at timestamptz NOT NULL DEFAULT now(),temporary_expires_at timestamptz,
 failed_attempts integer NOT NULL DEFAULT 0 CHECK(failed_attempts>=0),locked_until timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE iam.organizations (
 id uuid PRIMARY KEY,slug text NOT NULL UNIQUE CHECK(slug ~ '^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$'),
 name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 100),
 status text NOT NULL CHECK(status IN ('active','suspended')),
 authz_version bigint NOT NULL DEFAULT 1 CHECK(authz_version>0),
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE iam.memberships (
 id uuid PRIMARY KEY,org_id uuid NOT NULL REFERENCES iam.organizations(id),user_id uuid NOT NULL REFERENCES iam.users(id),
 status text NOT NULL CHECK(status IN ('active','disabled')),
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(org_id,user_id),UNIQUE(org_id,id)
);
CREATE INDEX memberships_user_status ON iam.memberships(user_id,status);
CREATE TABLE iam.roles (
 id uuid PRIMARY KEY,org_id uuid NOT NULL REFERENCES iam.organizations(id),
 code text NOT NULL CHECK(code ~ '^[a-z][a-z0-9_]{2,63}$'),
 name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 100),description text NOT NULL DEFAULT '' CHECK(char_length(description)<=1000),
 kind text NOT NULL CHECK(kind IN ('system','custom')),status text NOT NULL CHECK(status IN ('active','disabled')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),created_by uuid REFERENCES iam.users(id),
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(org_id,code),UNIQUE(org_id,id),
 CHECK((kind='system' AND code IN ('org_admin','member')) OR (kind='custom' AND code NOT IN ('org_admin','member','super_admin')))
);
CREATE TABLE iam.permissions (
 key text PRIMARY KEY CHECK(char_length(key)<=128),module text NOT NULL,resource_type text NOT NULL,action text NOT NULL,description text NOT NULL,
 scope text NOT NULL CHECK(scope IN ('platform','organization')),risk_level text NOT NULL CHECK(risk_level IN ('normal','sensitive','high')),
 delegable boolean NOT NULL,requires_recent_auth boolean NOT NULL,status text NOT NULL CHECK(status IN ('active','deprecated','retired')),
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE iam.role_permissions (
 org_id uuid NOT NULL,role_id uuid NOT NULL,permission_key text NOT NULL REFERENCES iam.permissions(key),
 created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(role_id,permission_key),
 FOREIGN KEY(org_id,role_id) REFERENCES iam.roles(org_id,id)
);
CREATE INDEX role_permissions_permission ON iam.role_permissions(permission_key,role_id);
-- +goose StatementBegin
CREATE FUNCTION iam.check_organization_permission() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM iam.permissions WHERE key=NEW.permission_key AND scope='organization' AND status='active') THEN
  RAISE EXCEPTION 'only active organization permissions can be granted' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER role_permissions_scope BEFORE INSERT OR UPDATE ON iam.role_permissions FOR EACH ROW EXECUTE FUNCTION iam.check_organization_permission();
CREATE TABLE iam.membership_roles (
 org_id uuid NOT NULL,membership_id uuid NOT NULL,role_id uuid NOT NULL,created_by uuid NOT NULL REFERENCES iam.users(id),
 created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(membership_id,role_id),
 FOREIGN KEY(org_id,membership_id) REFERENCES iam.memberships(org_id,id),
 FOREIGN KEY(org_id,role_id) REFERENCES iam.roles(org_id,id)
);
CREATE INDEX membership_roles_role ON iam.membership_roles(org_id,role_id);
CREATE TABLE iam.user_sessions (
 id uuid PRIMARY KEY,user_id uuid NOT NULL REFERENCES iam.users(id),token_hash text NOT NULL UNIQUE,
 auth_version bigint NOT NULL CHECK(auth_version>0),state text NOT NULL CHECK(state IN ('active','revoked')),
 authenticated_at timestamptz NOT NULL,reauthenticated_at timestamptz,
 absolute_expires_at timestamptz NOT NULL,idle_expires_at timestamptz NOT NULL,last_seen_at timestamptz NOT NULL,
 revoked_at timestamptz,revoke_reason text CHECK(char_length(revoke_reason)<=100),client_ip inet,user_agent text NOT NULL CHECK(char_length(user_agent)<=512),created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(absolute_expires_at>authenticated_at),CHECK(idle_expires_at<=absolute_expires_at)
);
CREATE INDEX user_sessions_user_state ON iam.user_sessions(user_id,state);
CREATE INDEX user_sessions_absolute ON iam.user_sessions(absolute_expires_at);
CREATE TABLE iam.scs_sessions(token text PRIMARY KEY,data bytea NOT NULL,expiry timestamptz NOT NULL);
CREATE INDEX scs_sessions_expiry ON iam.scs_sessions(expiry);
CREATE TABLE iam.audit_events (
 id uuid PRIMARY KEY,occurred_at timestamptz NOT NULL DEFAULT now(),org_id uuid,
 actor_type text NOT NULL CHECK(actor_type IN ('user','system','deployment_operator')),actor_id uuid,actor_name text,
 action text NOT NULL,target_type text NOT NULL,target_id uuid,outcome text NOT NULL CHECK(outcome IN ('success','denied','failure')),
 reason_code text,request_id text NOT NULL,decision_id uuid,metadata jsonb NOT NULL DEFAULT '{}' CHECK(octet_length(metadata::text)<=8192),created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_org_time ON iam.audit_events(org_id,occurred_at,id);
CREATE INDEX audit_events_actor_time ON iam.audit_events(actor_id,occurred_at);
CREATE INDEX audit_events_request ON iam.audit_events(request_id);
-- +goose Down
-- No automatic destructive rollback: restore through a reviewed forward migration.
SELECT 1;
