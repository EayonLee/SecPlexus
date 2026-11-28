package event

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connection/clickhouse"
)

const (
	maxResponse = 1 << 20
)

// Store 使用 ClickHouse HTTP 协议读写事件并核对表定义。
type Store struct {
	address  *url.URL
	database string
	qualify  *strings.Replacer
	username string
	password string
	client   *http.Client
}

// NewStore 创建专用客户端。写入方须配置实际 ClickHouse 端点与请求期限。
func NewStore(address, database, username, password string, timeout time.Duration) (*Store, error) {
	endpoint, err := clickhouse.ParseEndpoint(address)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		return nil, errors.New("ClickHouse timeout must be positive")
	}
	if !clickhouse.DatabaseNameValid(database) {
		return nil, errors.New("invalid ClickHouse event database name")
	}
	identifier := `"` + database + `"`
	qualify := strings.NewReplacer(
		" spx.events", " "+identifier+".events",
		" spx.event_schema_migrations", " "+identifier+".event_schema_migrations",
		"database = 'spx'", "database = '"+database+"'",
		"database='spx'", "database='"+database+"'",
	)
	return &Store{address: endpoint, database: identifier, qualify: qualify, username: username, password: password, client: &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}, nil
}

// Close 关闭空闲连接。
func (s *Store) Close() { s.client.CloseIdleConnections() }

// Insert 同步写入已经投影的 JSONEachRow 批次。
func (s *Store) Insert(ctx context.Context, rows []byte) error {
	if len(rows) == 0 {
		return nil
	}
	data, err := s.query(ctx, "INSERT INTO spx.events FORMAT JSONEachRow", rows)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) != 0 {
		return errors.New("unexpected ClickHouse insert response")
	}
	return nil
}

// ReadEvent 读取半开时间范围内的指定事件。FINAL 合并重试产生的逻辑重复。
func (s *Store) ReadEvent(ctx context.Context, id uuid.UUID, from, to time.Time) (json.RawMessage, error) {
	const sql = `SELECT event_json FROM spx.events FINAL
WHERE event_time >= fromUnixTimestamp64Milli({start:Int64}, 'UTC')
  AND event_time < fromUnixTimestamp64Milli({end:Int64}, 'UTC')
  AND event_id = {id:UUID}
LIMIT 1 SETTINGS max_threads = 2, max_execution_time = 5 FORMAT TabSeparatedRaw`
	data, err := s.queryWithParams(ctx, sql, nil, url.Values{
		"param_id":    {id.String()},
		"param_start": {strconv.FormatInt(from.Add(time.Millisecond-time.Nanosecond).UnixMilli(), 10)},
		"param_end":   {strconv.FormatInt(to.Add(time.Millisecond-time.Nanosecond).UnixMilli(), 10)},
	})
	if err != nil {
		return nil, err
	}

	data = bytes.TrimSpace(data)
	if len(data) > 0 && !json.Valid(data) {
		return nil, errors.New("invalid event JSON returned by ClickHouse")
	}
	return json.RawMessage(data), nil
}

func (s *Store) query(ctx context.Context, sql string, body []byte) ([]byte, error) {
	return s.queryWithParams(ctx, sql, body, nil)
}

func (s *Store) queryWithParams(ctx context.Context, sql string, body []byte, queryParams url.Values) ([]byte, error) {
	endpoint := *s.address
	parameters := endpoint.Query()
	parameters.Set("query", s.qualify.Replace(sql))
	parameters.Set("wait_end_of_query", "1")
	parameters.Set("async_insert", "0")
	for name, values := range queryParams {
		parameters[name] = values
	}
	endpoint.RawQuery = parameters.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("could not build ClickHouse request")
	}
	if s.username != "" {
		req.SetBasicAuth(s.username, s.password)
	}
	res, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var transport *url.Error
		if errors.As(err, &transport) {
			return nil, fmt.Errorf("ClickHouse request: %w", transport.Err)
		}
		return nil, fmt.Errorf("ClickHouse request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil {
		return nil, fmt.Errorf("read ClickHouse response: %w", err)
	}
	if res.StatusCode != http.StatusOK || res.Header.Get("X-ClickHouse-Exception-Code") != "" {
		return nil, fmt.Errorf("ClickHouse request failed with HTTP %d", res.StatusCode)
	}
	if len(data) > maxResponse {
		return nil, errors.New("ClickHouse response exceeds limit")
	}
	return data, nil
}

// CheckPermissions 核对事件写入账号对实际事件表的读写权限，不写入探测数据。
func (s *Store) CheckPermissions(ctx context.Context) error {
	granted, err := s.query(ctx, "CHECK GRANT SELECT, INSERT ON spx.events", nil)
	if err != nil {
		return fmt.Errorf("check ClickHouse event table permissions: %w", err)
	}
	if string(bytes.TrimSpace(granted)) != "1" {
		return errors.New("ClickHouse event table requires SELECT and INSERT privileges")
	}
	return nil
}

// CheckSchema 只读核对实际表结构及永久保存策略。
func (s *Store) CheckSchema(ctx context.Context) error {
	expected, err := migrationV1Columns()
	if err != nil {
		return err
	}
	rows, err := s.query(ctx, `SELECT name, type, comment, default_kind, default_expression
FROM system.columns WHERE database = 'spx' AND table = 'events' ORDER BY position FORMAT JSONEachRow`, nil)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(rows))
	for _, col := range expected {
		var actual struct {
			Name, Type, Comment string
			DefaultKind         string `json:"default_kind"`
			DefaultExpression   string `json:"default_expression"`
		}
		if err := decoder.Decode(&actual); err != nil {
			return fmt.Errorf("missing ClickHouse column %s", col.Name)
		}
		if actual.Name != col.Name || actual.Type != col.Type || actual.Comment != col.Comment ||
			actual.DefaultKind != "" || actual.DefaultExpression != "" {
			return fmt.Errorf("ClickHouse column %s differs: expected %s (%s), got %s (%s)", col.Name, col.Type, col.Comment, actual.Name, actual.Type)
		}
	}
	var actual struct {
		Name, Type, Comment string
		DefaultKind         string `json:"default_kind"`
		DefaultExpression   string `json:"default_expression"`
	}
	if err := decoder.Decode(&actual); err != nil || actual.Name != "parse_outcome" ||
		actual.Type != "LowCardinality(String)" || actual.Comment != "解析结果：ok、partial 或 error" ||
		actual.DefaultKind != "DEFAULT" || actual.DefaultExpression != "'ok'" {
		return errors.New("ClickHouse parse_outcome column differs")
	}
	if decoder.Decode(new(json.RawMessage)) != io.EOF {
		return errors.New("unexpected columns in the ClickHouse events table")
	}
	raw, err := s.query(ctx, `SELECT engine_full, partition_key, sorting_key, primary_key, create_table_query
FROM system.tables WHERE database='spx' AND name='events' FORMAT JSONEachRow`, nil)
	if err != nil {
		return err
	}
	var table struct {
		Engine       string `json:"engine_full"`
		PartitionKey string `json:"partition_key"`
		SortingKey   string `json:"sorting_key"`
		PrimaryKey   string `json:"primary_key"`
		Create       string `json:"create_table_query"`
	}
	if err := json.Unmarshal(raw, &table); err != nil {
		return fmt.Errorf("decode ClickHouse table definition: %w", err)
	}
	const order = "class_uid,event_time,event_id"
	engine, _, _ := strings.Cut(table.Engine, " ")
	if engine != "ReplacingMergeTree" || compactSQL(table.PartitionKey) != "toYYYYMM(event_time)" ||
		compactSQL(table.SortingKey) != order || compactSQL(table.PrimaryKey) != order {
		return fmt.Errorf("ClickHouse engine, partition or sort keys differ")
	}
	create := strings.ReplaceAll(compactSQL(table.Create), "`", "")
	if strings.Count(table.Create, " CONSTRAINT ") != 1 ||
		!strings.Contains(create, "CONSTRAINTevent_parse_outcomeCHECKparse_outcomeIN('ok','partial','error')") {
		return errors.New("ClickHouse parse_outcome constraint differs")
	}
	if ttlClause.MatchString(table.Create) {
		return errors.New("ClickHouse event table must not have a TTL")
	}
	return nil
}

func compactSQL(sql string) string { return strings.Join(strings.Fields(sql), "") }

var ttlClause = regexp.MustCompile(`\bTTL[[:space:]]`)
