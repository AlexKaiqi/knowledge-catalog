package scale

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"kc/index"
	"kc/kernel"
	"kc/reader"
	"kc/repository"

	"github.com/redis/go-redis/v9"
)

const (
	EnvRedisPassword = "KC_REDIS_PASSWORD"
	defaultRedisHost = "127.0.0.1"
	defaultRedisPort = 16379
)

// RedisConfig is non-secret Redis location. Target use is discardable hot-tail
// cache, not a warehouse and not a comparison engine. Password is never a field to persist.
type RedisConfig struct {
	Host     string `json:"host,omitempty" yaml:"host,omitempty"`
	Port     int    `json:"port,omitempty" yaml:"port,omitempty"`
	DB       int    `json:"db,omitempty" yaml:"db,omitempty"`
	Password string `json:"password,omitempty" yaml:"password,omitempty"`
}

// WithDefaults fills the local sandbox host/port. It does not set a password.
func (c RedisConfig) WithDefaults() RedisConfig {
	if c.Host == "" {
		c.Host = defaultRedisHost
	}
	if c.Port == 0 {
		c.Port = defaultRedisPort
	}
	return c
}

// RejectSecrets refuses passwords in the config file / URL / Password field.
func (c RedisConfig) RejectSecrets() error {
	if err := repository.RejectConfiguredSecret("redis", c.Host, EnvRedisPassword); err != nil {
		return err
	}
	if strings.TrimSpace(c.Password) != "" {
		return fmt.Errorf("redis connection config must not contain secrets; set %s", EnvRedisPassword)
	}
	return nil
}

func (c RedisConfig) addr() string {
	c = c.WithDefaults()
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// ParseRedisAddr reads host:port or redis://host:port/db without a password.
func ParseRedisAddr(raw string) (RedisConfig, error) {
	if err := repository.RejectConfiguredSecret("redis", raw, EnvRedisPassword); err != nil {
		return RedisConfig{}, err
	}
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return RedisConfig{}, err
		}
		if u.Scheme != "redis" && u.Scheme != "rediss" {
			return RedisConfig{}, fmt.Errorf("redis URL must be redis://host:port")
		}
		cfg := RedisConfig{Host: u.Hostname()}
		if p := u.Port(); p != "" {
			port, err := strconv.Atoi(p)
			if err != nil {
				return RedisConfig{}, fmt.Errorf("redis port: %w", err)
			}
			cfg.Port = port
		}
		if db := strings.TrimPrefix(u.Path, "/"); db != "" {
			n, err := strconv.Atoi(db)
			if err != nil {
				return RedisConfig{}, fmt.Errorf("redis db: %w", err)
			}
			cfg.DB = n
		}
		return cfg, nil
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		return RedisConfig{}, fmt.Errorf("redis address must be host:port")
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return RedisConfig{}, fmt.Errorf("redis port: %w", err)
	}
	return RedisConfig{Host: host, Port: n}, nil
}

// OpenRedis is a disposable Redis client used by tests. Target layer is
// hot-tail cache (scale `cache: redis`), not `index: redis`, not comparison,
// and not a warehouse. COMMIT / READ / APPEND do not land here.
//
// Args:
//
//	cfg: non-secret host/port/db. Empty fields use sandbox defaults. Password is KC_REDIS_PASSWORD.
//
// Returns:
//
//	an opener that builds one key prefix per repository.
func OpenRedis(cfg RedisConfig) index.EngineOpener {
	return func(_ string, id kernel.RepositoryID) (index.Engine, error) {
		if err := cfg.RejectSecrets(); err != nil {
			return nil, err
		}
		cfg = cfg.WithDefaults()
		cli := redis.NewClient(&redis.Options{
			Addr:        cfg.addr(),
			Password:    strings.TrimSpace(os.Getenv(EnvRedisPassword)),
			DB:          cfg.DB,
			DialTimeout: 3 * time.Second,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := cli.Ping(ctx).Err(); err != nil {
			_ = cli.Close()
			return nil, fmt.Errorf("redis ping: %w", err)
		}
		return &redisEngine{cli: cli, pfx: "kc:" + index.SanitizeID(string(id)) + ":"}, nil
	}
}

type redisEngine struct {
	cli *redis.Client
	pfx string
}

func (e *redisEngine) Close() error { return e.cli.Close() }

func (e *redisEngine) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 12*time.Second)
}

func (e *redisEngine) metaKey() string { return e.pfx + "meta" }
func (e *redisEngine) objsKey() string { return e.pfx + "objs" }
func (e *redisEngine) textKey() string { return e.pfx + "text" }
func (e *redisEngine) fieldsKey(oid string) string {
	return e.pfx + "f:" + oid
}
func (e *redisEngine) eqKey(path, value string) string {
	return e.pfx + "eq:" + path + "\x1f" + value
}
func (e *redisEngine) hasKey(path string) string { return e.pfx + "has:" + path }
func (e *redisEngine) numKey(path string) string { return e.pfx + "num:" + path }

func (e *redisEngine) LoadMeta() (index.Meta, error) {
	ctx, cancel := e.ctx()
	defer cancel()
	m, err := e.cli.HGetAll(ctx, e.metaKey()).Result()
	if err != nil {
		return index.Meta{}, err
	}
	return index.Meta{
		Basis:  kernel.CommitID(m["basis"]),
		Digest: kernel.Digest(m["digest"]),
		Mode:   m["mode"],
		Cause:  m["cause"],
	}, nil
}

func (e *redisEngine) putMeta(ctx context.Context, meta index.Meta) error {
	return e.cli.HSet(ctx, e.metaKey(), map[string]any{
		"basis":  string(meta.Basis),
		"digest": string(meta.Digest),
		"mode":   meta.Mode,
		"cause":  meta.Cause,
	}).Err()
}

func (e *redisEngine) wipe(ctx context.Context) error {
	var cursor uint64
	for {
		keys, next, err := e.cli.Scan(ctx, cursor, e.pfx+"*", 256).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := e.cli.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

func (e *redisEngine) Rebuild(docs []index.CompiledDoc, meta index.Meta) error {
	ctx, cancel := e.ctx()
	defer cancel()
	if err := e.wipe(ctx); err != nil {
		return err
	}
	for _, doc := range docs {
		if err := e.putDoc(ctx, doc); err != nil {
			return err
		}
	}
	return e.putMeta(ctx, meta)
}

func (e *redisEngine) Apply(upserts []index.CompiledDoc, deletes []kernel.ObjectID, meta index.Meta) error {
	ctx, cancel := e.ctx()
	defer cancel()
	for _, id := range deletes {
		if err := e.dropDoc(ctx, string(id)); err != nil {
			return err
		}
	}
	for _, doc := range upserts {
		if err := e.dropDoc(ctx, string(doc.ObjectID)); err != nil {
			return err
		}
		if err := e.putDoc(ctx, doc); err != nil {
			return err
		}
	}
	return e.putMeta(ctx, meta)
}

func (e *redisEngine) Count() (int, error) {
	ctx, cancel := e.ctx()
	defer cancel()
	n, err := e.cli.SCard(ctx, e.objsKey()).Result()
	return int(n), err
}

func (e *redisEngine) dropDoc(ctx context.Context, oid string) error {
	fields, err := e.cli.HGetAll(ctx, e.fieldsKey(oid)).Result()
	if err != nil {
		return err
	}
	pipe := e.cli.Pipeline()
	for path, value := range fields {
		pipe.SRem(ctx, e.eqKey(path, value), oid)
		pipe.SRem(ctx, e.hasKey(path), oid)
		pipe.ZRem(ctx, e.numKey(path), oid)
	}
	pipe.Del(ctx, e.fieldsKey(oid))
	pipe.HDel(ctx, e.textKey(), oid)
	pipe.SRem(ctx, e.objsKey(), oid)
	_, err = pipe.Exec(ctx)
	return err
}

func (e *redisEngine) putDoc(ctx context.Context, doc index.CompiledDoc) error {
	oid := string(doc.ObjectID)
	pipe := e.cli.Pipeline()
	pipe.SAdd(ctx, e.objsKey(), oid)
	pipe.HSet(ctx, e.textKey(), oid, doc.Text)
	for _, pair := range doc.Fields {
		path, value := pair[0], pair[1]
		pipe.HSet(ctx, e.fieldsKey(oid), path, value)
		pipe.SAdd(ctx, e.eqKey(path, value), oid)
		pipe.SAdd(ctx, e.hasKey(path), oid)
		if n, ok := parseIndexNumber(value); ok {
			pipe.ZAdd(ctx, e.numKey(path), redis.Z{Score: n, Member: oid})
		}
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (e *redisEngine) Search(req reader.SearchRequest, spec reader.IndexSpec) ([]kernel.ObjectID, error) {
	ctx, cancel := e.ctx()
	defer cancel()
	var sets [][]kernel.ObjectID
	var sortClause reader.SearchClause
	for _, c := range req.Clauses {
		if c.Op == reader.OpSort {
			sortClause = c
			continue
		}
		ids, err := e.clauseIDs(ctx, c, spec)
		if err != nil {
			return nil, err
		}
		sets = append(sets, ids)
	}
	if len(sets) == 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "search requires a locating clause")
	}
	ids := intersectIDs(sets)
	if sortClause.Path == "" {
		return ids, nil
	}
	field, _ := spec.Field(sortClause.Path)
	return e.orderIDs(ctx, ids, sortClause.Path, strings.EqualFold(sortClause.Order, "desc"), reader.NumericType(field.Type))
}

func (e *redisEngine) clauseIDs(ctx context.Context, c reader.SearchClause, spec reader.IndexSpec) ([]kernel.ObjectID, error) {
	switch c.Op {
	case reader.OpMatch:
		return e.matchIDs(ctx, c.Path, c.Value)
	case reader.OpEQ:
		return e.smembers(ctx, e.eqKey(c.Path, c.Value))
	case reader.OpIN:
		return e.inIDs(ctx, c.Path, c.Values)
	case reader.OpNEQ:
		return e.members(ctx, e.cli.SDiff(ctx, e.objsKey(), e.eqKey(c.Path, c.Value)))
	case reader.OpExists:
		return e.smembers(ctx, e.hasKey(c.Path))
	case reader.OpGT, reader.OpGTE, reader.OpLT, reader.OpLTE:
		field, _ := spec.Field(c.Path)
		return e.compareIDs(ctx, c, reader.NumericType(field.Type))
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "redis projection does not implement %s", c.Op)
	}
}

func (e *redisEngine) smembers(ctx context.Context, key string) ([]kernel.ObjectID, error) {
	return e.members(ctx, e.cli.SMembers(ctx, key))
}

func (e *redisEngine) members(_ context.Context, cmd *redis.StringSliceCmd) ([]kernel.ObjectID, error) {
	raw, err := cmd.Result()
	if err != nil {
		return nil, err
	}
	out := make([]kernel.ObjectID, 0, len(raw))
	for _, id := range raw {
		out = append(out, kernel.ObjectID(id))
	}
	return out, nil
}

func (e *redisEngine) inIDs(ctx context.Context, path string, values []string) ([]kernel.ObjectID, error) {
	if len(values) == 0 {
		return nil, nil
	}
	keys := make([]string, len(values))
	for i, v := range values {
		keys[i] = e.eqKey(path, v)
	}
	return e.members(ctx, e.cli.SUnion(ctx, keys...))
}

func (e *redisEngine) matchIDs(ctx context.Context, path, query string) ([]kernel.ObjectID, error) {
	tokens := matchTokens(query)
	if len(tokens) == 0 {
		return nil, nil
	}
	if path == "" {
		all, err := e.cli.HGetAll(ctx, e.textKey()).Result()
		if err != nil {
			return nil, err
		}
		var ids []kernel.ObjectID
		for oid, text := range all {
			if tokensIn(text, tokens) {
				ids = append(ids, kernel.ObjectID(oid))
			}
		}
		return ids, nil
	}
	oids, err := e.smembers(ctx, e.hasKey(path))
	if err != nil {
		return nil, err
	}
	var ids []kernel.ObjectID
	for _, oid := range oids {
		val, err := e.cli.HGet(ctx, e.fieldsKey(string(oid)), path).Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return nil, err
		}
		if tokensIn(val, tokens) {
			ids = append(ids, oid)
		}
	}
	return ids, nil
}

func (e *redisEngine) compareIDs(ctx context.Context, c reader.SearchClause, numeric bool) ([]kernel.ObjectID, error) {
	if numeric {
		n, ok := parseIndexNumber(c.Value)
		if !ok {
			return nil, nil
		}
		min, max := redisRange(c.Op, n)
		raw, err := e.cli.ZRangeByScore(ctx, e.numKey(c.Path), &redis.ZRangeBy{Min: min, Max: max}).Result()
		if err != nil {
			return nil, err
		}
		out := make([]kernel.ObjectID, 0, len(raw))
		for _, id := range raw {
			out = append(out, kernel.ObjectID(id))
		}
		return out, nil
	}
	oids, err := e.smembers(ctx, e.hasKey(c.Path))
	if err != nil {
		return nil, err
	}
	var ids []kernel.ObjectID
	for _, oid := range oids {
		val, err := e.cli.HGet(ctx, e.fieldsKey(string(oid)), c.Path).Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return nil, err
		}
		if stringCompare(c.Op, val, c.Value) {
			ids = append(ids, oid)
		}
	}
	return ids, nil
}

func (e *redisEngine) orderIDs(ctx context.Context, ids []kernel.ObjectID, path string, desc, numeric bool) ([]kernel.ObjectID, error) {
	if len(ids) == 0 {
		return ids, nil
	}
	type pair struct {
		id  kernel.ObjectID
		num float64
		str string
	}
	pairs := make([]pair, 0, len(ids))
	for _, id := range ids {
		p := pair{id: id}
		val, err := e.cli.HGet(ctx, e.fieldsKey(string(id)), path).Result()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		if err == nil {
			p.str = val
			if numeric {
				p.num, _ = strconv.ParseFloat(val, 64)
			}
		}
		pairs = append(pairs, p)
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		if numeric {
			if desc {
				return pairs[i].num > pairs[j].num
			}
			return pairs[i].num < pairs[j].num
		}
		if desc {
			return pairs[i].str > pairs[j].str
		}
		return pairs[i].str < pairs[j].str
	})
	out := make([]kernel.ObjectID, len(pairs))
	for i, p := range pairs {
		out[i] = p.id
	}
	return out, nil
}

func intersectIDs(sets [][]kernel.ObjectID) []kernel.ObjectID {
	if len(sets) == 0 {
		return nil
	}
	counts := map[kernel.ObjectID]int{}
	order := []kernel.ObjectID{}
	for _, id := range sets[0] {
		if counts[id] == 0 {
			order = append(order, id)
		}
		counts[id] = 1
	}
	for i := 1; i < len(sets); i++ {
		next := map[kernel.ObjectID]struct{}{}
		for _, id := range sets[i] {
			next[id] = struct{}{}
		}
		var kept []kernel.ObjectID
		for _, id := range order {
			if _, ok := next[id]; ok && counts[id] == i {
				counts[id] = i + 1
				kept = append(kept, id)
			}
		}
		order = kept
	}
	return order
}

var redisTokenUnsafe = regexp.MustCompile(`[^a-zA-Z0-9_\p{L}]+`)

func matchTokens(text string) []string {
	var out []string
	for _, w := range strings.Fields(strings.ToLower(text)) {
		w = redisTokenUnsafe.ReplaceAllString(w, "")
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

func tokensIn(text string, tokens []string) bool {
	lower := strings.ToLower(text)
	for _, tok := range tokens {
		if !strings.Contains(lower, tok) {
			return false
		}
	}
	return true
}

func parseIndexNumber(raw string) (float64, bool) {
	n, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func redisRange(op reader.SearchOp, n float64) (min, max string) {
	s := strconv.FormatFloat(n, 'f', -1, 64)
	switch op {
	case reader.OpGT:
		return "(" + s, "+inf"
	case reader.OpGTE:
		return s, "+inf"
	case reader.OpLT:
		return "-inf", "(" + s
	case reader.OpLTE:
		return "-inf", s
	default:
		return s, s
	}
}

func stringCompare(op reader.SearchOp, left, right string) bool {
	cmp := strings.Compare(left, right)
	switch op {
	case reader.OpGT:
		return cmp > 0
	case reader.OpGTE:
		return cmp >= 0
	case reader.OpLT:
		return cmp < 0
	case reader.OpLTE:
		return cmp <= 0
	default:
		return false
	}
}
