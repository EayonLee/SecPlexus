package service

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// CreateOrganization 创建组织及其初始管理员。
func (s *Service) CreateOrganization(ctx context.Context, input OrganizationInput, requestID string) (string, error) {
	admin, err := uuid.Parse(input.AdminUserID)
	if err != nil {
		return "", repository.ErrInvalid
	}

	id := uuid.New()
	err = s.Manage(
		ctx,
		iamclient.PermissionIAMPlatformOrganizationsManage,
		"",
		requestID,
		"organization",
		&id,
		func(tx *gorm.DB, p iamclient.Principal, metadata map[string]any) error {
			user, err := repository.NewQueries(tx).LockUser(ctx, admin)
			if err != nil {
				return err
			}

			if user.Status != "active" {
				return repository.ErrInvalid
			}

			actor, err := uuid.Parse(p.UserID)
			if err != nil {
				return err
			}

			// 未指定父组织时默认挂到根 master：全局只允许一个根，
			// 新建组织一律是根的后代。
			var parentID *uuid.UUID
			if input.ParentID != nil && *input.ParentID != "" {
				parsed, err := uuid.Parse(*input.ParentID)
				if err != nil {
					return repository.ErrInvalid
				}

				var parentVersion int64
				if err = repository.LockOrganizationVersion(ctx, tx, parsed.String(), &parentVersion); err != nil {
					return repository.ErrInvalid
				}

				parentID = &parsed
			} else {
				var rootID string
				if err = tx.WithContext(ctx).Raw(`SELECT id::text FROM iam.organizations WHERE parent_id IS NULL`).Row().Scan(&rootID); err != nil {
					return err
				}

				parsed, err := uuid.Parse(rootID)
				if err != nil {
					return err
				}

				parentID = &parsed
			}

			return repository.CreateOrganization(ctx, tx, id, string(input.Slug), string(input.Name), parentID, admin, actor)
		},
		admin,
	)
	return id.String(), err
}

// UpdateOrganization 更新组织名称，可选改挂父组织。
//
// 改名不改变任何用户关系，因此不做管理员不变量检查：
// 任一组织缺管理员都不应阻塞其他组织的改名。
func (s *Service) UpdateOrganization(ctx context.Context, orgID string, input OrganizationUpdate, requestID string) error {
	return s.Manage(
		ctx,
		iamclient.PermissionIAMPlatformOrganizationsManage,
		"",
		requestID,
		"organization",
		uuidPointer(orgID),
		func(tx *gorm.DB, p iamclient.Principal, metadata map[string]any) error {
			var version int64
			if err := repository.LockOrganizationVersion(ctx, tx, orgID, &version); err != nil {
				return err
			}

			if version != input.AuthzEpoch {
				return repository.ErrConflict
			}

			reparent := input.ParentID != nil
			if reparent {
				if err := s.reparentOrganization(ctx, tx, orgID, *input.ParentID); err != nil {
					return err
				}

				metadata["parent_id"] = *input.ParentID
			}

			return repository.UpdateOrganization(ctx, tx, orgID, string(input.Name))
		},
	)
}

// reparentOrganization 校验并改挂父组织，随后在同一事务内重算整棵子树的 depth。
//
// depth 的重算放在服务层而非触发器：organization_tree_guard 是 BEFORE ROW 触发器，
// 若在其中发出递归 UPDATE 会再次触发自身。服务层递归 UPDATE 每行经触发器只做
// NEW.depth = parent.depth + 1，与递归结果一致，幂等收敛。
func (s *Service) reparentOrganization(ctx context.Context, tx *gorm.DB, orgID, parentID string) error {
	if parentID == orgID {
		return repository.ErrInvalid
	}

	if err := repository.LockParentCandidate(ctx, tx, parentID); err != nil {
		return err
	}

	if err := repository.UpdateOrganizationParent(ctx, tx, orgID, parentID); err != nil {
		return err
	}

	return repository.RecomputeSubtreeDepth(ctx, tx, orgID)
}

// DeleteOrganization 删除组织及其用户关系、角色与授权。
//
// 不要求先清空用户：清空最后一个管理员会违反管理员不变量，
// 与「删除需零用户」组合会让组织永久不可删除。
// 仍拒绝删除根组织与仍有子组织的组织，要求先处理子树。
func (s *Service) DeleteOrganization(ctx context.Context, orgID string, requestID string) error {
	return s.Manage(
		ctx,
		iamclient.PermissionIAMPlatformOrganizationsManage,
		"",
		requestID,
		"organization",
		uuidPointer(orgID),
		func(tx *gorm.DB, p iamclient.Principal, metadata map[string]any) error {
			isRoot, err := repository.IsRootOrganization(ctx, tx, orgID)
			if err != nil {
				return err
			}

			if isRoot {
				return repository.ErrRootOrganization
			}

			children, err := repository.OrganizationChildCount(ctx, tx, orgID)
			if err != nil {
				return err
			}

			if children > 0 {
				return repository.ErrHasChildren
			}

			return repository.DeleteOrganization(ctx, tx, orgID)
		},
	)
}

// OrganizationTree 返回当前身份可见的组织树；truncated 表示超过上限被截断。
func (s *Service) OrganizationTree(ctx context.Context) ([]model.APIOrganization, bool, error) {
	if _, err := s.Authorize(ctx, iamclient.PermissionIAMOrganizationsRead, ""); err != nil {
		if _, platformErr := s.Authorize(ctx, iamclient.PermissionIAMPlatformOrganizationsRead, ""); platformErr != nil {
			return nil, false, err
		}
	}

	return repository.OrganizationTree(ctx, s.Store.DB)
}
