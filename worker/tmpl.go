package main

import (
	"encoding/binary"
	"fmt"
	mrand "math/rand"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Templating for request URL, body and header values. Each string is compiled
// ONCE (at worker start) into literal + generator segments; Render() is called
// per request and simply concatenates them, so a string with no generators is
// returned as-is with negligible overhead on the load-generation hot path.
//
// Recognized generators (anything else inside {{ }} is kept verbatim):
//   {{uuid}}              random UUID v4
//   {{seq}}              monotonic counter, shared across all workers (starts at 1)
//   {{now}}              current unix time in seconds
//   {{nowMs}}            current unix time in milliseconds
//   {{timestamp}}        current UTC time, RFC3339
//   {{randInt min max}}  random integer in [min, max]
//   {{randString n}}     random alphanumeric string of length n

// segment is one piece of a compiled template: literal text or a generator.
type segment struct {
	literal string
	gen     func() string
}

// Template is a precompiled string whose {{...}} generators expand per Render.
type Template struct {
	segs   []segment
	static bool
	raw    string
}

var seqCounter uint64

const alphanum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Compile parses s once into segments.
func Compile(s string) *Template {
	t := &Template{raw: s}
	if !strings.Contains(s, "{{") {
		t.static = true
		return t
	}

	i := 0
	for i < len(s) {
		open := strings.Index(s[i:], "{{")
		if open < 0 {
			t.segs = append(t.segs, segment{literal: s[i:]})
			break
		}
		open += i
		if open > i {
			t.segs = append(t.segs, segment{literal: s[i:open]})
		}
		rel := strings.Index(s[open+2:], "}}")
		if rel < 0 {
			// no closing braces: the rest is literal
			t.segs = append(t.segs, segment{literal: s[open:]})
			break
		}
		end := open + 2 + rel
		token := strings.TrimSpace(s[open+2 : end])
		if g := parseGen(token); g != nil {
			t.segs = append(t.segs, segment{gen: g})
		} else {
			// unknown token: keep the raw "{{...}}" so real content is not eaten
			t.segs = append(t.segs, segment{literal: s[open : end+2]})
		}
		i = end + 2
	}

	static := true
	for _, sg := range t.segs {
		if sg.gen != nil {
			static = false
			break
		}
	}
	t.static = static
	return t
}

// Render expands the template. Static templates return the original string.
func (t *Template) Render() string {
	if t.static {
		return t.raw
	}
	var b strings.Builder
	for _, sg := range t.segs {
		if sg.gen != nil {
			b.WriteString(sg.gen())
		} else {
			b.WriteString(sg.literal)
		}
	}
	return b.String()
}

// parseGen returns a generator for a recognized token, or nil.
func parseGen(tok string) func() string {
	fields := strings.Fields(tok)
	if len(fields) == 0 {
		return nil
	}
	switch fields[0] {
	case "uuid":
		return genUUID
	case "seq":
		return func() string { return strconv.FormatUint(atomic.AddUint64(&seqCounter, 1), 10) }
	case "now":
		return func() string { return strconv.FormatInt(time.Now().Unix(), 10) }
	case "nowMs":
		return func() string { return strconv.FormatInt(time.Now().UnixMilli(), 10) }
	case "timestamp":
		return func() string { return time.Now().UTC().Format(time.RFC3339) }
	case "randInt":
		if len(fields) != 3 {
			return nil
		}
		min, err1 := strconv.Atoi(fields[1])
		max, err2 := strconv.Atoi(fields[2])
		if err1 != nil || err2 != nil {
			return nil
		}
		return func() string { return strconv.Itoa(genRandInt(min, max)) }
	case "randString":
		if len(fields) != 2 {
			return nil
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil
		}
		return func() string { return genRandString(n) }
	}
	return nil
}

func genUUID() string {
	var b [16]byte
	binary.BigEndian.PutUint64(b[0:8], mrand.Uint64())
	binary.BigEndian.PutUint64(b[8:16], mrand.Uint64())
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func genRandInt(min, max int) int {
	if max < min {
		min, max = max, min
	}
	return min + mrand.Intn(max-min+1)
}

func genRandString(n int) string {
	if n <= 0 {
		return ""
	}
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = alphanum[mrand.Intn(len(alphanum))]
	}
	return string(buf)
}
