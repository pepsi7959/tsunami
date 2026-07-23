package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	tshttp "github.com/tsunami/libs"
	tsauth "github.com/tsunami/master/auth"
	tsgrpc "github.com/tsunami/proto"
)

// ---- topology graph for the web-control visualizer ----
type topoNode struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // master | worker | target
	IP       string `json:"ip"`
	Endpoint string `json:"endpoint"`
}
type topoEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // control | load
	Job  string `json:"job,omitempty"`
}
type topoGraph struct {
	Nodes []topoNode `json:"nodes"`
	Edges []topoEdge `json:"edges"`
}

func methodEnum(s string) tsgrpc.HTTPMethod {
	switch strings.ToUpper(s) {
	case "POST":
		return tsgrpc.HTTPMethod_POST
	case "PUT":
		return tsgrpc.HTTPMethod_PUT
	case "DELETE":
		return tsgrpc.HTTPMethod_DELETE
	case "UPDATE":
		return tsgrpc.HTTPMethod_UPDATE
	default:
		return tsgrpc.HTTPMethod_GET
	}
}

func confFromReq(c tshttp.CmdConf) tshttp.Conf {
	return tshttp.Conf{
		Name:        c.Name,
		URL:         c.URL,
		Protocol:    c.Protocol,
		Host:        c.Host,
		Port:        c.Port,
		Path:        c.Path,
		Method:      c.Method,
		Headers:     c.Headers,
		Body:        c.Body,
		Concurrence: c.Concurrence,
		Verbose:     c.Verbose,
	}
}

func startCommand(name string, conf tshttp.Conf, take int) *tsgrpc.OceanMessage {
	return &tsgrpc.OceanMessage{Msg: &tsgrpc.OceanMessage_Command{Command: &tsgrpc.Command{
		Action: tsgrpc.Action_START,
		Job:    name,
		Params: &tsgrpc.Params{
			Name:        name,
			Url:         conf.URL,
			Method:      methodEnum(conf.Method),
			Protocol:    conf.Protocol,
			Host:        conf.Host,
			Port:        conf.Port,
			Path:        conf.Path,
			Concurrency: int32(take),
			Body:        conf.Body,
			Verbose:     conf.Verbose,
			Header:      headersToProto(conf.Headers),
		},
	}}}
}

// headersToProto converts a header map to the proto header list.
func headersToProto(h map[string]string) []*tsgrpc.HTTPHeader {
	if len(h) == 0 {
		return nil
	}
	out := make([]*tsgrpc.HTTPHeader, 0, len(h))
	for k, v := range h {
		out = append(out, &tsgrpc.HTTPHeader{Key: k, Value: v})
	}
	return out
}

func stopCommand(name string) *tsgrpc.OceanMessage {
	return &tsgrpc.OceanMessage{Msg: &tsgrpc.OceanMessage_Command{Command: &tsgrpc.Command{
		Action: tsgrpc.Action_STOP, Job: name,
	}}}
}

// allocate distributes `requested` concurrency across attached workers, balanced
// and capped per worker. Returns nil,false if the fleet can't cover it (reject).
// Caller holds oc.mu.
func (oc *Ocean) allocate(requested int) (map[string]int, bool) {
	ids := make([]string, 0, len(oc.workers))
	avail := make(map[string]int, len(oc.workers))
	total := 0
	for id, w := range oc.workers {
		free := w.maxCap - w.used
		if free < 0 {
			free = 0
		}
		ids = append(ids, id)
		avail[id] = free
		total += free
	}
	if requested > total {
		return nil, false
	}
	sort.Strings(ids) // deterministic

	alloc := make(map[string]int, len(ids))
	remaining := requested
	for remaining > 0 {
		active := make([]string, 0, len(ids))
		for _, id := range ids {
			if avail[id]-alloc[id] > 0 {
				active = append(active, id)
			}
		}
		if len(active) == 0 {
			break
		}
		share := remaining / len(active)
		if share < 1 {
			share = 1 // distribute the remainder one at a time
		}
		for _, id := range active {
			if remaining <= 0 {
				break
			}
			give := share
			if room := avail[id] - alloc[id]; give > room {
				give = room
			}
			if give > remaining {
				give = remaining
			}
			alloc[id] += give
			remaining -= give
		}
	}
	if remaining > 0 {
		return nil, false
	}
	for id, c := range alloc {
		if c == 0 {
			delete(alloc, id)
		}
	}
	return alloc, true
}

func cors(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)
	}
}

// Options handles CORS preflight.
func (oc *Ocean) Options(w http.ResponseWriter, r *http.Request) {
	cors(w, r)
	w.Header().Set("Access-Control-Allow-Methods", "GET,PUT,POST,DELETE,OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Content-Length, X-Requested-With")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	tshttp.WriteSuccess(&w, nil, nil)
}

// --- user auth (login / logout / session) ---

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Provider string `json:"provider"` // "local" (default); "google" later
}

// Login authenticates via the selected provider and, on success, issues a
// session and sets the httpOnly session cookie.
func (oc *Ocean) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req loginReq
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}
	provName := req.Provider
	if provName == "" {
		provName = "local"
	}
	p := oc.auth.Provider(provName)
	if p == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 400, Message: "unknown auth provider"})
		return
	}
	u, err := p.Authenticate(map[string]string{"username": req.Username, "password": req.Password})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 401, Message: "invalid username or password"})
		return
	}
	raw, exp, err := oc.auth.Issue(u, r.RemoteAddr, r.UserAgent())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 500, Message: "could not create session"})
		return
	}
	http.SetCookie(w, oc.auth.Cookie(raw, exp))
	data := map[string]string{"email": u.Email, "name": u.Name}
	tshttp.WriteSuccess(&w, &data, nil)
}

// Logout revokes the current session and clears the cookie.
func (oc *Ocean) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	if c, err := r.Cookie(oc.auth.CookieName()); err == nil {
		oc.auth.Revoke(c.Value)
	}
	http.SetCookie(w, oc.auth.ClearCookie())
	tshttp.WriteSuccess(&w, nil, nil)
}

// Session returns the authenticated user (the web control calls it on boot to
// choose login-vs-app and to render the header username).
func (oc *Ocean) Session(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	u := tsauth.UserFrom(r.Context())
	if u == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 401, Message: "unauthorized"})
		return
	}
	data := map[string]string{"email": u.Email, "name": u.Name}
	tshttp.WriteSuccess(&w, &data, nil)
}

// --- saved tests (bookmarks) ---

// GetBookmarks returns all saved tests as a JSON string in data.bookmarks
// (same envelope pattern as GetInfo/GetSample).
func (oc *Ocean) GetBookmarks(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	list, err := oc.auth.Store().ListBookmarks()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 500, Message: "could not read bookmarks"})
		return
	}
	b, _ := json.Marshal(list)
	data := map[string]string{"bookmarks": string(b)}
	tshttp.WriteSuccess(&w, &data, nil)
}

// SaveBookmark upserts a saved test. Favoriting a running test sends only the
// name — the live job config is pulled from memory; editing sends the full conf.
func (oc *Ocean) SaveBookmark(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req tshttp.Request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}
	c := req.Conf
	if c.Name == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 400, Message: "name is required"})
		return
	}
	// favorite-from-running: only a name is given → fill from the live job config
	if c.URL == "" {
		oc.mu.Lock()
		if j := oc.jobs[c.Name]; j != nil {
			c.URL = j.rawConf.URL
			c.Method = j.rawConf.Method
			c.Concurrence = j.rawConf.Concurrence
			c.Body = j.rawConf.Body
			c.Headers = j.rawConf.Headers
			c.Verbose = j.rawConf.Verbose
		}
		oc.mu.Unlock()
	}
	bm := &tsauth.Bookmark{
		Name: c.Name, URL: c.URL, Method: c.Method, Concurrence: c.Concurrence,
		Body: c.Body, Headers: c.Headers, Verbose: c.Verbose,
	}
	if err := oc.auth.Store().SaveBookmark(bm); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 500, Message: "could not save bookmark"})
		return
	}
	tshttp.WriteSuccess(&w, nil, nil)
}

// DeleteBookmark removes a saved test (toggle-off favorite).
func (oc *Ocean) DeleteBookmark(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req tshttp.Request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}
	if req.Conf.Name == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 400, Message: "name is required"})
		return
	}
	if err := oc.auth.Store().DeleteBookmark(req.Conf.Name); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 500, Message: "could not delete bookmark"})
		return
	}
	tshttp.WriteSuccess(&w, nil, nil)
}

// --- environment presets (shared {{var}} sets) ---

// GetEnvs returns all environments as a JSON string in data.envs.
func (oc *Ocean) GetEnvs(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	list, err := oc.auth.Store().ListEnvs()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 500, Message: "could not read envs"})
		return
	}
	b, _ := json.Marshal(list)
	active, _ := oc.auth.Store().GetSetting("active_env")
	data := map[string]string{"envs": string(b), "active": active}
	tshttp.WriteSuccess(&w, &data, nil)
}

// SetActiveEnv persists the active environment applied by every start and
// resume, so the choice is shared and survives reloads / multiple clients.
func (oc *Ocean) SetActiveEnv(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req tshttp.Request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}
	if err := oc.auth.Store().SetSetting("active_env", strings.TrimSpace(req.Conf.Name)); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 500, Message: "could not set active env"})
		return
	}
	tshttp.WriteSuccess(&w, nil, nil)
}

// SaveEnv upserts an environment (name + variable map).
func (oc *Ocean) SaveEnv(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req struct {
		Conf struct {
			Name string            `json:"name"`
			Vars map[string]string `json:"vars"`
		} `json:"conf"`
	}
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}
	name := strings.TrimSpace(req.Conf.Name)
	if name == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 400, Message: "env name is required"})
		return
	}
	if err := oc.auth.Store().SaveEnv(&tsauth.Env{Name: name, Vars: req.Conf.Vars}); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 500, Message: "could not save env"})
		return
	}
	tshttp.WriteSuccess(&w, nil, nil)
}

// DeleteEnv removes an environment.
func (oc *Ocean) DeleteEnv(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req tshttp.Request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}
	if req.Conf.Name == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 400, Message: "name is required"})
		return
	}
	if err := oc.auth.Store().DeleteEnv(req.Conf.Name); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 500, Message: "could not delete env"})
		return
	}
	tshttp.WriteSuccess(&w, nil, nil)
}

// applyEnv substitutes {{key}} tokens in the URL, path, body and header values
// with the environment's variable values. Unknown {{...}} tokens are left intact
// so the worker's per-request generators ({{uuid}}, {{seq}}, ...) still expand.
func applyEnv(conf *tshttp.Conf, vars map[string]string) {
	if len(vars) == 0 {
		return
	}
	rep := func(s string) string {
		if s == "" {
			return s
		}
		for k, v := range vars {
			s = strings.ReplaceAll(s, "{{"+k+"}}", v)
		}
		return s
	}
	conf.URL = rep(conf.URL)
	conf.Path = rep(conf.Path)
	conf.Body = rep(conf.Body)
	for k, v := range conf.Headers {
		conf.Headers[k] = rep(v)
	}
}

// activeEnv returns the currently-selected environment name (persisted server
// side so start/resume always apply the same active env, for every client).
func (oc *Ocean) activeEnv() string {
	v, _ := oc.auth.Store().GetSetting("active_env")
	return v
}

// envVars returns the variable map for a named environment, or nil if the name
// is empty or the environment does not exist.
func (oc *Ocean) envVars(name string) map[string]string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if e, err := oc.auth.Store().GetEnv(name); err == nil && e != nil {
		return e.Vars
	}
	return nil
}

// confWithEnv returns a copy of conf (cloning the headers map so the caller's
// original is left untouched) with the environment's {{vars}} substituted. The
// untouched original keeps its {{tokens}}, so a different env can be applied
// later when a paused test is resumed.
func confWithEnv(conf tshttp.Conf, vars map[string]string) tshttp.Conf {
	out := conf
	if conf.Headers != nil {
		h := make(map[string]string, len(conf.Headers))
		for k, v := range conf.Headers {
			h[k] = v
		}
		out.Headers = h
	}
	applyEnv(&out, vars)
	return out
}

// Start splits a load test across attached workers (reject if capacity short).
func (oc *Ocean) Start(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req tshttp.Request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}
	name := req.Conf.Name
	// keep the raw conf ({{tokens}} intact) so a resume can re-apply whatever env
	// is active then; send the active env's substitution to the workers now.
	raw := confFromReq(req.Conf)
	conf := confWithEnv(raw, oc.envVars(oc.activeEnv()))
	requested := req.Conf.Concurrence

	oc.mu.Lock()
	if oc.jobs[name] != nil {
		oc.mu.Unlock()
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 409, Message: "a test named \"" + name + "\" is already running"})
		return
	}
	alloc, ok := oc.allocate(requested)
	if !ok {
		oc.mu.Unlock()
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 503, Message: "insufficient worker capacity for the requested concurrency"})
		return
	}
	// drop any cached metric/sample left over from a previous run of this name
	for _, wk := range oc.workers {
		delete(wk.metrics, name)
		delete(wk.samples, name)
	}
	for id, take := range alloc {
		wk := oc.workers[id]
		wk.used += take
		wk.command(startCommand(name, conf, take))
		fmt.Printf("assign %d to worker %s (job %s)\n", take, wk.name, name)
	}
	oc.jobs[name] = &job{name: name, conf: conf, rawConf: raw, assignments: alloc}
	oc.mu.Unlock()

	tshttp.WriteSuccess(&w, nil, nil)
}

// Stop stops a running test on all its workers and frees capacity.
func (oc *Ocean) Stop(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req tshttp.Request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}
	name := req.Conf.Name

	oc.mu.Lock()
	j := oc.jobs[name]
	if j == nil {
		oc.mu.Unlock()
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 404, Message: "service not found"})
		return
	}
	for id, take := range j.assignments {
		if wk := oc.workers[id]; wk != nil {
			wk.used -= take
			if wk.used < 0 {
				wk.used = 0
			}
			wk.command(stopCommand(name))
			// clear cached metric/sample so a stopped job leaves nothing behind
			delete(wk.metrics, name)
			delete(wk.samples, name)
		}
	}
	delete(oc.jobs, name)
	oc.mu.Unlock()

	tshttp.WriteSuccess(&w, nil, nil)
}

// Pause halts a test's traffic but KEEPS its concurrency reserved (unlike Stop):
// wk.used and the job entry are left intact, so Resume returns to the same
// concurrency and no other test can claim the slots. The worker tears the load
// down, so the job's stats reset (rebuilt fresh on Resume).
func (oc *Ocean) Pause(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req tshttp.Request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}
	name := req.Conf.Name

	oc.mu.Lock()
	j := oc.jobs[name]
	if j == nil {
		oc.mu.Unlock()
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 404, Message: "service not found"})
		return
	}
	for id := range j.assignments {
		if wk := oc.workers[id]; wk != nil {
			wk.command(stopCommand(name)) // halt traffic; capacity stays reserved
			delete(wk.metrics, name)      // reset stats (rebuilt on resume)
			delete(wk.samples, name)
		}
	}
	j.paused = true
	oc.mu.Unlock()

	tshttp.WriteSuccess(&w, nil, nil)
}

// Resume restarts a paused test on its held workers at the same concurrency.
func (oc *Ocean) Resume(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req tshttp.Request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}
	name := req.Conf.Name

	oc.mu.Lock()
	j := oc.jobs[name]
	if j == nil {
		oc.mu.Unlock()
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 404, Message: "service not found"})
		return
	}
	// re-apply the current active env to the raw conf so continue always runs
	// with the latest selected environment.
	conf := confWithEnv(j.rawConf, oc.envVars(oc.activeEnv()))
	for id, take := range j.assignments {
		if wk := oc.workers[id]; wk != nil {
			wk.command(startCommand(j.name, conf, take))
		}
	}
	j.conf = conf
	j.paused = false
	oc.mu.Unlock()

	tshttp.WriteSuccess(&w, nil, nil)
}

// GetMetrics aggregates the latest per-worker metrics for a running test.
func (oc *Ocean) GetMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req tshttp.Request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}
	name := req.Conf.Name

	oc.mu.Lock()
	j := oc.jobs[name]
	if j == nil {
		oc.mu.Unlock()
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 404, Message: name + " not running"})
		return
	}
	agg := tshttp.Metric{Name: name}
	var avgWeighted, weight float64
	var merged []int64 // fleet-merged latency histogram (per bucket)
	for id := range j.assignments {
		wk := oc.workers[id]
		if wk == nil {
			continue
		}
		m := wk.metrics[name]
		if m == nil {
			continue
		}
		for i, c := range m.GetBucket() {
			for i >= len(merged) {
				merged = append(merged, 0)
			}
			merged[i] += c
		}
		agg.WorkerCount += int(m.GetWorkerCount())
		agg.RequestCount += int(m.GetRequestCount())
		agg.ErrorCount += int(m.GetErrorCount())
		agg.Rps += m.GetRps()
		if m.GetMax() > agg.Max {
			agg.Max = m.GetMax()
		}
		if m.GetMin() > 0 && (agg.Min == 0.0 || m.GetMin() < agg.Min) {
			agg.Min = m.GetMin()
		}
		if m.GetElapsedTime() > agg.ElapedTime {
			agg.ElapedTime = m.GetElapsedTime()
		}
		// request-weighted mean latency; weight by successful responses (req-err)
		if wgt := float64(m.GetRequestCount() - m.GetErrorCount()); wgt > 0 {
			avgWeighted += m.GetAvg() * wgt
			weight += wgt
		}
	}
	oc.mu.Unlock()
	if weight > 0 {
		agg.Avg = avgWeighted / weight
	}
	data := metricxToSlice(&agg)
	// cumulative fleet histogram for the web control (per-interval percentiles +
	// latency heatmap are derived client-side by diffing successive snapshots)
	b, _ := json.Marshal(merged)
	(*data)["buckets"] = string(b)
	tshttp.WriteSuccess(&w, data, nil)
}

func metricxToSlice(m *tshttp.Metric) *map[string]string {
	data := map[string]string{
		"name":         m.Name,
		"avg":          fmt.Sprintf("%f", m.Avg),
		"min":          fmt.Sprintf("%f", m.Min),
		"max":          fmt.Sprintf("%f", m.Max),
		"error_count":  fmt.Sprintf("%d", m.ErrorCount),
		"workerCount":  fmt.Sprintf("%d", m.WorkerCount),
		"ElapedTime":   fmt.Sprintf("%f", m.ElapedTime),
		"RequestCount": fmt.Sprintf("%d", m.RequestCount),
		"rps":          fmt.Sprintf("%f", m.Rps),
	}
	return &data
}

// SampleView is a JSON-friendly last request/response captured by a worker.
type SampleView struct {
	Worker          string            `json:"worker"`
	WorkerID        string            `json:"workerID"`
	Method          string            `json:"method"`
	URL             string            `json:"url"`
	RequestHeaders  map[string]string `json:"requestHeaders"`
	RequestBody     string            `json:"requestBody"`
	Status          int               `json:"status"`
	ResponseHeaders map[string]string `json:"responseHeaders"`
	ResponseBody    string            `json:"responseBody"`
	LatencyMs       float64           `json:"latencyMs"`
	Error           string            `json:"error"`
	TS              int64             `json:"ts"`
}

func headerMap(hs []*tsgrpc.HTTPHeader) map[string]string {
	m := make(map[string]string, len(hs))
	for _, h := range hs {
		m[h.GetKey()] = h.GetValue()
	}
	return m
}

// GetSample returns the latest request/response each worker captured for a
// verbose job (verbose must have been enabled when the test was started).
func (oc *Ocean) GetSample(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req tshttp.Request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}
	job := req.Conf.Name

	views := make([]SampleView, 0)
	oc.mu.Lock()
	for _, wk := range oc.workers {
		s := wk.samples[job]
		if s == nil {
			continue
		}
		views = append(views, SampleView{
			Worker:          wk.name,
			WorkerID:        wk.id,
			Method:          s.GetMethod(),
			URL:             s.GetUrl(),
			RequestHeaders:  headerMap(s.GetRequestHeader()),
			RequestBody:     s.GetRequestBody(),
			Status:          int(s.GetStatusCode()),
			ResponseHeaders: headerMap(s.GetResponseHeader()),
			ResponseBody:    s.GetResponseBody(),
			LatencyMs:       s.GetLatencyMs(),
			Error:           s.GetError(),
			TS:              s.GetTs(),
		})
	}
	oc.mu.Unlock()

	b, _ := json.Marshal(views)
	tshttp.WriteSuccess(&w, &map[string]string{"job": job, "samples": string(b)}, nil)
}

// GetInfo reports cluster state + topology for the web control.
func (oc *Ocean) GetInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req tshttp.Request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}

	data := make(map[string]string)
	oc.mu.Lock()

	maxConcurrent, used := 0, 0
	oceanIP := ""
	if ip := tshttp.GetIP(); ip != nil {
		oceanIP = ip.String()
	}
	g := topoGraph{}
	g.Nodes = append(g.Nodes, topoNode{ID: "master:" + oc.id, Name: oc.name, Kind: "master", IP: oceanIP, Endpoint: oc.viper.GetString("endpoints.http")})

	for id, wk := range oc.workers {
		maxConcurrent += wk.maxCap
		used += wk.used
		data["worker_"+id+"_max_qouta"] = fmt.Sprintf("%d", wk.maxCap)
		data["worker_"+id+"_remain_qouta"] = fmt.Sprintf("%d", wk.maxCap-wk.used)
		g.Nodes = append(g.Nodes, topoNode{ID: id, Name: wk.name, Kind: "worker", IP: wk.ip, Endpoint: wk.ip})
		g.Edges = append(g.Edges, topoEdge{From: "master:" + oc.id, To: id, Kind: "control"})
	}

	names := make([]string, 0, len(oc.jobs))
	paused := make([]string, 0)
	seenTarget := map[string]bool{}
	for name, j := range oc.jobs {
		names = append(names, name)
		if j.paused {
			paused = append(paused, name)
		}
		url := j.conf.URL
		if url != "" {
			tid := "target:" + url
			if !seenTarget[tid] {
				seenTarget[tid] = true
				g.Nodes = append(g.Nodes, topoNode{ID: tid, Name: url, Kind: "target", Endpoint: url})
			}
			for id := range j.assignments {
				g.Edges = append(g.Edges, topoEdge{From: id, To: tid, Kind: "load", Job: name})
			}
		}
	}

	data["name"] = oc.name
	data["id"] = oc.id
	data["workers"] = fmt.Sprintf("%d", len(oc.workers))
	data["max_connections"] = fmt.Sprintf("%d", oc.maxConnections)
	data["max_concurrent"] = fmt.Sprintf("%d", maxConcurrent)
	data["remaining_Concurrent"] = fmt.Sprintf("%d", maxConcurrent-used)
	data["paused_jobs"] = strings.Join(paused, ",")
	oc.mu.Unlock()

	// fleet merge: show peer oceans + all running jobs when discovery is enabled
	if oc.discovery {
		for _, o := range oc.allOceans() {
			if o.ID == oc.id {
				continue
			}
			g.Nodes = append(g.Nodes, topoNode{ID: "master:" + o.ID, Name: o.Name, Kind: "master", IP: o.GRPC, Endpoint: o.HTTP})
		}
		for _, fj := range oc.allFleetJobs() {
			// this ocean's own jobs are already in `names` from oc.jobs (authoritative +
			// current); skip its fleet entry, which is only published periodically and is
			// stale right after a stop (otherwise a stopped job lingers until the next publish).
			if fj.Ocean == oc.id {
				continue
			}
			found := false
			for _, n := range names {
				if n == fj.Name {
					found = true
					break
				}
			}
			if !found {
				names = append(names, fj.Name)
			}
			if fj.Target != "" && !seenTarget["target:"+fj.Target] {
				seenTarget["target:"+fj.Target] = true
				g.Nodes = append(g.Nodes, topoNode{ID: "target:" + fj.Target, Name: fj.Target, Kind: "target", Endpoint: fj.Target})
			}
		}
	}
	data["jobs"] = strings.Join(names, ",")

	if b, err := json.Marshal(g); err == nil {
		data["topology"] = string(b)
	}
	tshttp.WriteSuccess(&w, &data, nil)
}
