package pipeline

import (
	"strings"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
)

func (j Job) mixedResult() (mapped normalizer.Result, fields map[string]any, outcome Outcome) {
	failed := func(stage Stage, reason Reason) normalizer.Result {
		outcome = Outcome{Status: StatusError, Stage: stage, Reason: reason}
		base := normalizer.BaseResult(j.Received)
		base.Issues = []normalizer.Issue{{Code: string(reason)}}
		return base
	}
	defer func() {
		if recover() != nil {
			mapped = failed(StageMap, ReasonEnginePanic)
			fields = nil
		}
	}()
	engine := j.Source.Mapping
	if engine == nil {
		mapped = failed(StageClassify, ReasonNoRule)
		return
	}
	var err error
	switch {
	case j.Meta.DecodeFailed:
		mapped = failed(StageDecode, ReasonFormatMismatch)
		return
	case j.Fields == nil && j.Meta.Decoded:
		fields, err = engine.ParseDecodedFields(j.Raw)
	case j.Fields == nil:
		fields, err = engine.ParseFields(j.Raw)
	case j.Source.Protocol == input.ProtocolSyslog && engine.Mapping().Parse.Mode == parser.ModeCEF:
		fields = make(map[string]any, len(j.Fields))
		for key, value := range j.Fields {
			if name, ok := strings.CutPrefix(key, "cef."); ok {
				fields[name] = value
			}
		}
		if len(fields) == 0 {
			mapped = failed(StageDecode, ReasonFormatMismatch)
			return
		}
	case j.Source.Protocol == input.ProtocolSyslog:
		message, _ := j.Fields["message"].(string)
		fields, err = engine.ParseDecodedFields([]byte(message))
	default:
		fields = j.Fields
	}
	if err != nil {
		mapped = failed(StageDecode, ReasonFormatMismatch)
		return
	}
	mapped, fields, err = engine.ApplyFieldsWithExtraction(fields, j.Received)
	if err != nil {
		mapped = failed(StageMap, ReasonContractViolation)
		return
	}

	outcome = Outcome{Status: StatusOK}
	if engine.Classifies() && mapped.BranchID == "" {
		outcome = Outcome{Status: StatusError, Stage: StageClassify, Reason: ReasonNoMatchingBranch}
	}
	if j.Meta.Truncated {
		mapped.Issues = append(mapped.Issues, normalizer.Issue{Code: string(ReasonTruncated)})
	}
	if len(mapped.Issues) > 0 && outcome.OK() {
		outcome = Outcome{Status: StatusPartial, Stage: StageMap, Reason: ReasonContractViolation}
	}
	return
}

func (j Job) mixedEvent(event Event) (Event, map[string]any, error) {
	mapped, fields, outcome := j.mixedResult()
	event.Outcome = outcome
	event.Truncated, event.OriginalBytes = j.Meta.Truncated, j.Meta.OriginalBytes
	event.BranchID, event.CandidateClass, event.Issues = mapped.BranchID, mapped.CandidateClass, mapped.Issues
	event.TimeSource = mapped.TimeSource
	event.AppliedDefaults = mapped.AppliedDefaults
	event.PartitionKey = partitionKey(mapped.Fields, event.EventID)
	body, err := buildBody(j, mapped.Fields, mapped.Unmapped, event)
	if err != nil {
		event.Outcome = Outcome{Status: StatusError, Stage: StageMap, Reason: ReasonContractViolation}
		event.Issues = []normalizer.Issue{{Code: string(ReasonContractViolation)}}
		event.TimeSource, event.PartitionKey = "received", event.EventID
		body, err = buildBody(j, normalizer.BaseResult(j.Received).Fields, nil, event)
	}
	event.Body = body
	return event, fields, err
}

// PreviewSource 用接入点上下文和实际解析路径试算已为 UTF-8 的样本文本。
func PreviewSource(source input.Entry, sample string, received time.Time) (Event, map[string]any, error) {
	raw := []byte(sample)
	job := Job{Source: source, Raw: raw, Received: received, Meta: Meta{Decoded: true}}
	event, fields, err := job.mixedEvent(Event{EventID: normalizer.EventID(source.ID, [16]byte{}, 0, raw)})
	event.Body = normalizer.PreviewEvent(event.Body)
	return event, normalizer.PreviewFields(fields), err
}
