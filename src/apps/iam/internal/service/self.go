package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
)

// Identity 返回当前身份与可见组织的权限投影。
func (s *Service) Identity(ctx context.Context, orgID string) (map[string]any, error) {
	p, err := s.Sessions.Resolve(ctx)
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"principal":             p,
		"organizations":         []any{},
		"platform_capabilities": []string{},
		"capabilities":          []string{},
	}

	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		return nil, err
	}

	rows, err := repository.VisibleOrganizations(ctx, s.Store.DB, userID, p.IsSuperAdmin)
	if err != nil {
		return nil, err
	}

	organizations := []map[string]string{}
	for rows.Next() {
		var id, slug, name, version string
		if err = rows.Scan(&id, &slug, &name, &version); err != nil {
			rows.Close()
			return nil, err
		}

		organizations = append(organizations, map[string]string{"id": id, "slug": slug, "name": name, "authz_version": version})
	}

	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}

	result["organizations"] = organizations
	catalog, err := repository.Permissions(ctx, s.Store.DB)
	if err != nil {
		return nil, err
	}

	platform := []string{}
	for _, permission := range catalog {
		if permission.Scope == "platform" && p.IsSuperAdmin && permission.Status != "retired" {
			platform = append(platform, permission.Key)
		}
	}

	result["platform_capabilities"] = platform
	if orgID != "" {
		allowed := false
		for _, org := range organizations {
			if org["id"] == orgID {
				allowed = true
			}
		}

		if !allowed {
			return nil, authorization.ErrDenied
		}

		org, err := uuid.Parse(orgID)
		if err != nil {
			return nil, repository.ErrInvalid
		}

		capabilities, err := repository.NewQueries(s.Store.DB).GetOrganizationPermissions(ctx, repository.GetOrganizationPermissionsParams{OrgID: org,
			UserID: userID})
		if err != nil {
			return nil, err
		}

		if p.IsSuperAdmin {
			capabilities = []string{}
			for _, permission := range catalog {
				if permission.Scope == "organization" && permission.Status == "active" {
					capabilities = append(capabilities, permission.Key)
				}
			}
		}

		result["capabilities"] = capabilities
		result["org_id"] = orgID
	}

	return result, nil
}

// SessionsList 返回当前用户仍有效的会话，不暴露令牌摘要。
func (s *Service) SessionsList(ctx context.Context) (map[string]any, error) {
	p, err := s.Sessions.Resolve(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := repository.ActiveSessions(ctx, s.Store.DB, p.UserID, p.AuthVersion)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var id, agent, ip string
		var authenticated, seen, idle, absolute time.Time
		if err = rows.Scan(&id, &authenticated, &seen, &idle, &absolute, &agent, &ip); err != nil {
			return nil, err
		}

		items = append(items, map[string]any{
			"id":                  id,
			"authenticated_at":    authenticated,
			"last_seen_at":        seen,
			"idle_expires_at":     idle,
			"absolute_expires_at": absolute,
			"user_agent":          agent,
			"client_ip":           ip,
			"current":             id == p.SessionID,
		})
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return map[string]any{"items": items}, nil
}
