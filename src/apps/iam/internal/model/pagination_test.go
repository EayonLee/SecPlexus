package model

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestListQueryDefaultsToTenAndHonorsExplicitPageSize(t *testing.T) {
	for _, entry := range []struct {
		query         string
		limit, offset int
	}{{"", 10, 0}, {"?limit=20&offset=40", 20, 40}} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/"+entry.query, nil)
		var input ListQuery
		if err := c.ShouldBindQuery(&input); err != nil {
			t.Fatal(err)
		}
		if input.Limit != entry.limit || input.Offset != entry.offset {
			t.Fatalf("pagination: got %d/%d, want %d/%d", input.Limit, input.Offset, entry.limit, entry.offset)
		}
	}
}
