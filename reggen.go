// Package reggen generates text based on regex definitions
package reggen

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"regexp/syntax"
	"strings"
	"time"
)

const runeRangeEnd = 0x10ffff
const printableChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~ \t\n\r"

var printableCharsNoNL = printableChars[:len(printableChars)-2]

type state struct {
	limit    int
	minTotal int // minimum total output length (0 = no minimum)
}

type Generator struct {
	re    *syntax.Regexp
	rand  *rand.Rand
	debug bool
}

func (g *Generator) generate(s *state, re *syntax.Regexp, b *strings.Builder) {
	op := re.Op
	switch op {
	case syntax.OpNoMatch:
	case syntax.OpEmptyMatch:
	case syntax.OpLiteral:
		for _, r := range re.Rune {
			b.WriteRune(r)
		}
	case syntax.OpCharClass:
		// number of possible chars
		sum := 0
		for i := 0; i < len(re.Rune); i += 2 {
			sum += int(re.Rune[i+1]-re.Rune[i]) + 1
			if re.Rune[i+1] == runeRangeEnd {
				sum = -1
				break
			}
		}
		// pick random char in range (inverse match group)
		if sum == -1 {
			possibleChars := []uint8{}
			for j := 0; j < len(printableChars); j++ {
				c := printableChars[j]
				for i := 0; i < len(re.Rune); i += 2 {
					if rune(c) >= re.Rune[i] && rune(c) <= re.Rune[i+1] {
						possibleChars = append(possibleChars, c)
						break
					}
				}
			}
			if len(possibleChars) > 0 {
				c := possibleChars[g.rand.Intn(len(possibleChars))]
				b.WriteByte(c)
			}

			return
		}

		r := g.rand.Intn(int(sum))
		var ru rune
		sum = 0
		for i := 0; i < len(re.Rune); i += 2 {
			gap := int(re.Rune[i+1]-re.Rune[i]) + 1
			if sum+gap > r {
				ru = re.Rune[i] + rune(r-sum)
				break
			}
			sum += gap
		}

		b.WriteRune(ru)
	case syntax.OpAnyCharNotNL, syntax.OpAnyChar:
		chars := printableChars
		if op == syntax.OpAnyCharNotNL {
			chars = printableCharsNoNL
		}
		c := chars[g.rand.Intn(len(chars))]
		b.WriteByte(c)
	case syntax.OpBeginLine:
	case syntax.OpEndLine:
	case syntax.OpBeginText:
	case syntax.OpEndText:
	case syntax.OpWordBoundary:
	case syntax.OpNoWordBoundary:
	case syntax.OpCapture:
		g.generate(s, re.Sub0[0], b)
	case syntax.OpStar:
		// Repeat zero or more times.
		// Cap repetitions to avoid pathological blowup on patterns like
		// `(-[\w]+)*` with limit=1011. The cap keeps generation O(cap)
		// per quantifier regardless of the overall length limit.
		hi := s.limit
		if hi > 50 {
			hi = 50
		}

		lo := 0
		if s.minTotal > 0 && s.minTotal <= hi {
			lo = g.rand.Intn(s.minTotal + 1)
		}

		count := lo + g.rand.Intn(hi-lo+1)
		for i := 0; i < count; i++ {
			for _, r := range re.Sub {
				g.generate(s, r, b)
			}
		}
	case syntax.OpPlus:
		// Repeat one or more times.
		hi := s.limit
		if hi > 50 {
			hi = 50
		}

		lo := 1
		if s.minTotal > 1 && s.minTotal <= hi {
			lo = 1 + g.rand.Intn(s.minTotal)
		}

		count := lo + g.rand.Intn(hi-lo+1)
		for i := 0; i < count; i++ {
			for _, r := range re.Sub {
				g.generate(s, r, b)
			}
		}
	case syntax.OpQuest:
		// Zero or one instances
		count := g.rand.Intn(2)
		for i := 0; i < count; i++ {
			for _, r := range re.Sub {
				g.generate(s, r, b)
			}
		}
	case syntax.OpRepeat:
		re.Max = int(math.Min(float64(re.Max), float64(s.limit)))
		count := 0
		if re.Max > re.Min {
			count = g.rand.Intn(re.Max - re.Min + 1)
		}
		for i := 0; i < re.Min || i < (re.Min+count); i++ {
			for _, r := range re.Sub {
				g.generate(s, r, b)
			}
		}
	case syntax.OpConcat:
		for _, r := range re.Sub {
			g.generate(s, r, b)
		}
	case syntax.OpAlternate:
		i := g.rand.Intn(len(re.Sub))
		g.generate(s, re.Sub[i], b)
	default:
		fmt.Fprintln(os.Stderr, "[reg-gen] Unhandled op: ", op)
	}
}

// limit is the maximum number of times star, range or plus should repeat
// i.e. [0-9]+ will generate at most 10 characters if this is set to 10
func (g *Generator) Generate(limit int) string {
	var b strings.Builder
	g.generate(&state{limit: limit}, g.re, &b)
	return b.String()
}

// DefaultMaxAttempts is the default number of attempts GenerateWithLength
// makes before returning the best-effort result. Kept low to avoid
// dominating CPU when many patterns are generated per spec — real-world
// patterns with built-in quantifiers ({n,m}) usually hit on the first
// few attempts.
const DefaultMaxAttempts = 20

// GenerateWithLength generates a string matching the regex with a target
// length between minLen and maxLen. Uses minLen as a repetition floor for
// unbounded quantifiers (*, +) to bias generation toward the target length,
// then retries if the output doesn't meet the constraints.
func (g *Generator) GenerateWithLength(minLen, maxLen int) string {
	return g.GenerateWithLengthN(minLen, maxLen, DefaultMaxAttempts)
}

// GenerateWithLengthN is like GenerateWithLength but allows specifying the
// maximum number of attempts before returning the best-effort result.
func (g *Generator) GenerateWithLengthN(minLen, maxLen, maxAttempts int) string {
	var b strings.Builder
	st := &state{limit: maxLen, minTotal: minLen}

	for range maxAttempts {
		b.Reset()
		g.generate(st, g.re, &b)
		n := len([]rune(b.String()))
		if n >= minLen && n <= maxLen {
			return b.String()
		}
	}

	// Fallback: return whatever we get.
	b.Reset()
	g.generate(st, g.re, &b)
	return b.String()
}

// create a new generator
func NewGenerator(regex string) (*Generator, error) {
	re, err := syntax.Parse(regex, syntax.Perl)
	if err != nil {
		return nil, err
	}
	return &Generator{
		re:   re,
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

func (gen *Generator) SetSeed(seed int64) {
	gen.rand = rand.New(rand.NewSource(seed))
}

func Generate(regex string, limit int) (string, error) {
	g, err := NewGenerator(regex)
	if err != nil {
		return "", err
	}
	return g.Generate(limit), nil
}
