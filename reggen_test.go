package reggen

import (
	"fmt"
	"regexp"
	"testing"
	"time"
)

type testCase struct {
	regex string
}

var cases = []testCase{
	{`123[0-2]+.*\w{3}`},
	{`^\d{1,2}[/](1[0-2]|[1-9])[/]((19|20)\d{2})$`},
	{`^((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])$`},
	{`^\d+$`},
	{`\D{3}`},
	{`((123)?){3}`},
	{`(ab|bc)def`},
	{`[^abcdef]{5}`},
	{`[^1]{3,5}`},
	{`[[:upper:]]{5}`},
	{`[^0-5a-z\s]{5}`},
	{`Z{2,5}`},
	{`[a-zA-Z]{100}`},
	{`^[a-z]{5,10}@[a-z]{5,10}\.(com|net|org)$`},
}

func TestGenerate(t *testing.T) {
	for _, test := range cases {
		for i := 0; i < 10; i++ {
			r, err := NewGenerator(test.regex)
			if err != nil {
				t.Fatal("Error creating generator: ", err)
			}
			r.debug = false
			res := r.Generate(10)
			// only print first result
			if i < 1 {
				fmt.Printf("Regex: %v Result: \"%s\"\n", test.regex, res)
			}
			re, err := regexp.Compile(test.regex)
			if err != nil {
				t.Fatal("Invalid test case. regex: ", test.regex, " failed to compile:", err)
			}
			if !re.MatchString(res) {
				t.Error("Generated data does not match regex. Regex: ", test.regex, " output: ", res)
			}
		}
	}
}

func TestSeed(t *testing.T) {
	g1, err := NewGenerator(cases[0].regex)
	if err != nil {
		t.Fatal("Error creating generator: ", err)
	}
	g2, err := NewGenerator(cases[0].regex)
	if err != nil {
		t.Fatal("Error creating generator: ", err)
	}
	currentTime := time.Now().UnixNano()
	g1.SetSeed(currentTime)
	g2.SetSeed(currentTime)
	for i := 0; i < 10; i++ {
		if g1.Generate(100) != g2.Generate(100) {
			t.Error("Results are not reproducible")
		}
	}

	g1.SetSeed(123)
	g2.SetSeed(456)
	for i := 0; i < 10; i++ {
		if g1.Generate(100) == g2.Generate(100) {
			t.Error("Results should not match")
		}
	}

}

func TestGenerateWithLength(t *testing.T) {
	cases := []struct {
		regex  string
		minLen int
		maxLen int
	}{
		// Kendra IndexId pattern: exactly 36 chars
		{`[a-zA-Z0-9][a-zA-Z0-9-]*`, 36, 36},
		// Kendra ExperienceId: 1-36 chars
		{`[a-zA-Z0-9][a-zA-Z0-9_-]*`, 1, 36},
		// Simple digit pattern with exact length
		{`\d+`, 10, 10},
		// UUID-like uppercase
		{`[0-9A-F]+`, 32, 32},
		// Range
		{`[a-z]+`, 5, 20},
	}

	for _, tc := range cases {
		t.Run(tc.regex, func(t *testing.T) {
			g, err := NewGenerator(tc.regex)
			if err != nil {
				t.Fatal("Error creating generator:", err)
			}
			g.SetSeed(42)

			re, err := regexp.Compile(tc.regex)
			if err != nil {
				t.Fatal("Invalid regex:", err)
			}

			hits := 0
			for i := 0; i < 100; i++ {
				s := g.GenerateWithLength(tc.minLen, tc.maxLen)
				n := len([]rune(s))
				if n >= tc.minLen && n <= tc.maxLen {
					hits++
				}
				if !re.MatchString(s) {
					t.Errorf("iteration %d: %q does not match %s", i, s, tc.regex)
				}
			}
			// Require at least 80% hit rate — the bias should make most
			// attempts land within bounds. Exact-length patterns with
			// unbounded quantifiers have inherently lower hit rates.
			if hits < 80 {
				t.Errorf("hit rate too low: %d/100 (want >= 80%%)", hits)
			}
		})
	}
}

func BenchmarkGenerate(b *testing.B) {
	r, err := NewGenerator(`^[a-z]{5,10}@[a-z]+\.(com|net|org)$`)
	if err != nil {
		b.Fatal("Error creating generator: ", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Generate(10)
	}
}

func BenchmarkGenerateWithLength(b *testing.B) {
	cases := []struct {
		name    string
		pattern string
		minLen  int
		maxLen  int
	}{
		{"simple_charclass_exact", `[a-zA-Z0-9][a-zA-Z0-9-]*`, 36, 36},
		{"arn_pattern", `arn:aws(-[\w]+)*:[a-z0-9-\\.]{0,63}:[a-z0-9-\\.]{0,63}:[0-9]{12}:(service|vpc)/[A-Za-z0-9*-]+`, 1, 1011},
		{"uuid", `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}`, 36, 36},
		{"simple_bounded", `[A-Za-z0-9][A-Za-z0-9\-_]{3,31}`, 4, 32},
		{"dot_star", `.*`, 0, 51200},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			g, err := NewGenerator(tc.pattern)
			if err != nil {
				b.Fatal(err)
			}
			g.SetSeed(42)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				g.GenerateWithLength(tc.minLen, tc.maxLen)
			}
		})
	}
}
