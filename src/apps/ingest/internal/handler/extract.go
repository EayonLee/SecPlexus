package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/validation"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// ExtractMapping 提取并安全回显样本的字段，无需创建规则。
func (a *Admin) ExtractMapping(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, validation.MaxSampleRequestBytes)
	var request struct {
		normalizer.Extraction
		Samples []string `json:"samples"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		writeMappingError(c.Writer, err)
		return
	}
	if err := validation.CheckSamples(request.Samples); err != nil {
		writeMappingError(c.Writer, err)
		return
	}
	engine, err := normalizer.CompileExtraction(request.Extraction)
	if err != nil {
		result.Success(c.Writer, sampleResponse{Total: len(request.Samples), CompileErr: err.Error()})
		return
	}
	result.Success(c.Writer, extractSamples(c.Request.Context(), engine, request.Samples))
}

type extractionSample struct {
	Index     int                `json:"index"`
	Fields    map[string]any     `json:"fields"`
	Types     map[string]string  `json:"field_types,omitempty"`
	Reason    string             `json:"reason,omitempty"`
	Issues    []normalizer.Issue `json:"issues,omitempty"`
	Truncated bool               `json:"truncated,omitempty"`
}

type extractionResponse struct {
	Compiled bool               `json:"compiled"`
	Previews []extractionSample `json:"previews"`
}

func extractSamples(ctx context.Context, engine *normalizer.Compiled, samples []string) extractionResponse {
	response := extractionResponse{Compiled: true, Previews: make([]extractionSample, 0, len(samples))}
	for index, sample := range samples {
		if ctx.Err() != nil {
			break
		}
		entry := extractionSample{Index: index}
		fields, err := engine.ParseDecodedFields([]byte(sample))
		if err != nil {
			entry.Reason = string(pipeline.DecodeFailureReason(err))
		} else {
			fields, entry.Issues = engine.ProcessFields(fields)
			entry.Fields = normalizer.PreviewFields(fields)
			entry.Types = normalizer.PreviewFieldTypes(fields, entry.Fields)
			entry.Truncated = len(entry.Fields) < len(fields)
		}
		response.Previews = append(response.Previews, entry)
	}
	return response
}
