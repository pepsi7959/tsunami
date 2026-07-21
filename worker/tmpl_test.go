package main

import (
	"regexp"
	"strconv"
	"testing"
)

func TestStaticPassthrough(t *testing.T) {
	tpl := Compile(`{"user":"alice","n":1}`)
	if !tpl.static {
		t.Fatal("expected static template")
	}
	if got := tpl.Render(); got != `{"user":"alice","n":1}` {
		t.Fatalf("static render changed: %q", got)
	}
}

func TestUnknownTokenKeptLiteral(t *testing.T) {
	tpl := Compile(`a {{bogus}} b {{randInt}} c`) // randInt with no args is invalid -> literal
	got := tpl.Render()
	if got != `a {{bogus}} b {{randInt}} c` {
		t.Fatalf("unknown tokens should pass through, got %q", got)
	}
}

func TestUUIDAndBody(t *testing.T) {
	tpl := Compile(`{"id":"{{uuid}}"}`)
	re := regexp.MustCompile(`^\{"id":"[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}"\}$`)
	got := tpl.Render()
	if !re.MatchString(got) {
		t.Fatalf("uuid body did not match: %q", got)
	}
	// two renders should differ
	if tpl.Render() == got {
		t.Fatalf("expected different uuid on second render")
	}
}

func TestSeqIncrements(t *testing.T) {
	tpl := Compile(`{{seq}}`)
	a, _ := strconv.Atoi(tpl.Render())
	b, _ := strconv.Atoi(tpl.Render())
	if b != a+1 {
		t.Fatalf("seq should increment: %d then %d", a, b)
	}
}

func TestRandIntInRange(t *testing.T) {
	tpl := Compile(`{{randInt 5 7}}`)
	for i := 0; i < 200; i++ {
		n, err := strconv.Atoi(tpl.Render())
		if err != nil || n < 5 || n > 7 {
			t.Fatalf("randInt out of range: %v (err %v)", n, err)
		}
	}
}

func TestRandStringLength(t *testing.T) {
	tpl := Compile(`p_{{randString 8}}`)
	got := tpl.Render()
	if len(got) != len("p_")+8 {
		t.Fatalf("randString length wrong: %q", got)
	}
}

func TestMixedURL(t *testing.T) {
	tpl := Compile(`https://api/x?rid={{uuid}}&n={{seq}}`)
	if tpl.static {
		t.Fatal("should not be static")
	}
	got := tpl.Render()
	if matched, _ := regexp.MatchString(`^https://api/x\?rid=[0-9a-f-]+&n=\d+$`, got); !matched {
		t.Fatalf("mixed url render wrong: %q", got)
	}
}
