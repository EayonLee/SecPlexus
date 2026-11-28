package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/schema/events"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// EventSchema 提供只读的事件类及字段查询。
type EventSchema struct {
	Catalog *events.Catalog
}

// List 返回版本和事件类摘要。
func (h EventSchema) List(c *gin.Context) {
	catalog := h.catalog(c)
	if catalog == nil {
		return
	}
	result.Success(c.Writer, struct {
		events.Version
		Classes []events.Class `json:"classes"`
	}{Version: catalog.Version, Classes: catalog.Classes()})
}

// Fields 返回去重的完整字段目录以及每一类的适用定义。
func (h EventSchema) Fields(c *gin.Context) {
	catalog := h.catalog(c)
	if catalog == nil {
		return
	}
	result.Success(c.Writer, struct {
		events.Version
		Fields []events.CatalogField `json:"fields"`
	}{Version: catalog.Version, Fields: catalog.Fields()})
}

// GetClass 返回一个事件类在当前契约中的字段。
func (h EventSchema) GetClass(c *gin.Context) {
	uid, err := strconv.Atoi(c.Param("class_uid"))
	if err != nil || uid < 0 {
		result.Fail(c.Writer, result.CodeInvalidRequest)
		return
	}
	catalog := h.catalog(c)
	if catalog == nil {
		return
	}
	class, ok := catalog.Class(uid)
	if !ok {
		result.Fail(c.Writer, result.CodeNotFound)
		return
	}
	result.Success(c.Writer, struct {
		events.Version
		events.Class
	}{Version: catalog.Version, Class: class})
}

func (h EventSchema) catalog(c *gin.Context) *events.Catalog {
	version := c.Query("contract_version")
	if version == "" {
		return h.Catalog
	}
	catalog, err := events.LoadVersion(version)
	if err != nil {
		result.Fail(c.Writer, result.CodeNotFound)
		return nil
	}
	return catalog
}
