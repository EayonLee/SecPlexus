// Package service 负责 Ingest 的用例编排与业务校验。
package service

import (
	"context"
	"slices"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/validation"
)

// RuleStore 提供保存解析规则及检查当前修订所需的存储能力。
type RuleStore interface {
	ReadMapping(context.Context, string) (repository.Mapping, error)
	CreateMapping(context.Context, repository.CreateMappingParams) (repository.Mapping, error)
	UpdateMapping(context.Context, string, repository.CreateMappingParams) (repository.Mapping, error)
}

// SaveRule 核对当前修订与样本覆盖，然后提交规则和验证记录。
func SaveRule(ctx context.Context, store RuleStore, request SaveRuleInput) (repository.Mapping, error) {
	var current repository.Mapping
	if request.ID != "" {
		var err error
		current, err = store.ReadMapping(ctx, request.ID)
		if err != nil {
			return repository.Mapping{}, err
		}
		if current.Builtin {
			return repository.Mapping{}, repository.ErrNotFound
		}
		if int(request.ExpectedVersion) != current.Version {
			return repository.Mapping{}, repository.ErrParsingRuleVersion
		}
	}

	params, engine, err := PrepareRuleWrite(request)
	if err != nil {
		return repository.Mapping{}, err
	}
	params.Verification, err = verifyRuleWrite(ctx, params, engine, current, request.Samples)
	if err != nil {
		return repository.Mapping{}, err
	}
	if request.ID == "" {
		return store.CreateMapping(ctx, params)
	}
	return store.UpdateMapping(ctx, request.ID, params)
}

// PrepareRuleWrite 校验并归一化待保存的规则，供新建、编辑与恢复使用。
func PrepareRuleWrite(request SaveRuleInput) (repository.CreateMappingParams, *normalizer.Compiled, error) {
	params := repository.CreateMappingParams{
		Name: request.Name, Vendor: request.Vendor, Product: request.Product,
		ExpectedVersion: request.ExpectedVersion, AuthorID: request.AuthorID,
	}
	for _, item := range []struct {
		field validation.Field
		value *string
	}{{validation.Name, &params.Name}, {validation.Vendor, &params.Vendor}, {validation.Product, &params.Product}} {
		value, err := validation.RuleText(item.field, *item.value)
		if err != nil {
			return params, nil, err
		}
		*item.value = value
	}
	definition, engine, err := normalizer.PrepareDefinition(request.Definition)
	if err != nil {
		return params, nil, err
	}
	if len(definition) > 4<<20 {
		return params, nil, &validation.Issue{Field: "definition", Code: "too_long", Limit: 4 << 20}
	}
	params.Definition = definition
	return params, engine, nil
}

func verifyRuleWrite(ctx context.Context, params repository.CreateMappingParams, engine *normalizer.Compiled, current repository.Mapping, samples []string) (*validation.RuleVerification, error) {
	definition, vendor, product := params.Definition, params.Vendor, params.Product
	required, err := RuleBranchesToVerify(current, definition, vendor, product)
	if err != nil {
		return nil, err
	}
	if len(samples) > 0 || len(required) > 0 {
		if err := validation.CheckSamples(samples); err != nil {
			return nil, err
		}
	}
	if len(required) == 0 {
		return current.Verification, nil
	}

	digest, err := normalizer.ExecutionDigest(definition, vendor, product)
	if err != nil {
		return nil, err
	}
	source := input.Entry{Mapping: engine, Vendor: vendor, Product: product}
	verified := validation.RuleVerification{Digest: digest, VerifiedAt: time.Now().UTC(), SampleCount: len(samples)}
	for _, sample := range samples {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		event, _, err := pipeline.PreviewSource(source, sample, verified.VerifiedAt)
		if err != nil || event.Outcome.Status != pipeline.StatusOK {
			continue
		}
		verified.Passed++
		if !slices.Contains(verified.CoveredBranches, event.BranchID) {
			verified.CoveredBranches = append(verified.CoveredBranches, event.BranchID)
		}
	}
	for _, branch := range required {
		if !slices.Contains(verified.CoveredBranches, branch) {
			return nil, &validation.Issue{Field: "samples", Code: "insufficient_coverage"}
		}
	}
	slices.Sort(verified.CoveredBranches)
	return &verified, nil
}

// RuleBranchesToVerify 确定当前来源条件下需要重新试算的分支。
func RuleBranchesToVerify(current repository.Mapping, definition []byte, vendor, product string) ([]string, error) {
	prior := current.Spec
	if current.Verification == nil || current.Vendor != vendor || current.Product != product {
		prior = nil
	}
	return normalizer.BranchesToVerify(prior, definition)
}
