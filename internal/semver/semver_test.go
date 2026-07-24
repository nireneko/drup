package semver

import (
	"testing"
)

func TestParse_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  Version
	}{
		{"1.2.3", Version{1, 2, 3}},
		{"0.0.0", Version{0, 0, 0}},
		{"10.3.0", Version{10, 3, 0}},
		{"8.4.2", Version{8, 4, 2}},
		{"11.0.0", Version{11, 0, 0}},
		// Missing patch defaults to 0.
		{"8.1", Version{8, 1, 0}},
		{"11.0", Version{11, 0, 0}},
		// With v prefix.
		{"v1.2.3", Version{1, 2, 3}},
		// With pre-release suffix (ignored for comparison).
		{"1.2.3-beta1", Version{1, 2, 3}},
		// Single number (major only).
		{"11", Version{11, 0, 0}},
		{"10", Version{10, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParse_Invalid(t *testing.T) {
	tests := []string{
		"",
		"abc",
		"1.2.3.4",
		"1.",
		".1.2",
		"1..2",
	}
	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			_, err := Parse(tt)
			if err == nil {
				t.Errorf("Parse(%q) should return error", tt)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.1.0", "1.0.0", 1},
		{"1.0.1", "1.0.0", 1},
		{"8.3.0", "8.4.0", -1},
		{"10.3.0", "11.0.0", -1},
		// Pre-release ignored.
		{"1.0.0-beta", "1.0.0", 0},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			va, _ := Parse(tt.a)
			vb, _ := Parse(tt.b)
			got := va.Compare(vb)
			if got != tt.want {
				t.Errorf("(%s).Compare(%s) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSatisfies_GreaterOrEqual(t *testing.T) {
	tests := []struct {
		version    string
		constraint string
		want       bool
	}{
		{"8.3.0", ">=8.1", true},
		{"8.1.0", ">=8.1", true},
		{"8.0.0", ">=8.1", false},
		{"8.4.0", ">=8.1", true},
		{"7.4.0", ">=8.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.version+"_"+tt.constraint, func(t *testing.T) {
			v, err := Parse(tt.version)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.version, err)
			}
			got := Satisfies(v, tt.constraint)
			if got != tt.want {
				t.Errorf("Satisfies(%s, %q) = %v, want %v", tt.version, tt.constraint, got, tt.want)
			}
		})
	}
}

func TestSatisfies_Caret(t *testing.T) {
	tests := []struct {
		version    string
		constraint string
		want       bool
	}{
		// ^8.1 means >=8.1.0, <9.0.0
		{"8.2.0", "^8.1", true},
		{"8.1.0", "^8.1", true},
		{"9.0.0", "^8.1", false},
		{"8.0.0", "^8.1", false},
		// ^10.3 means >=10.3.0, <11.0.0
		{"10.3.0", "^10.3", true},
		{"10.5.0", "^10.3", true},
		{"11.0.0", "^10.3", false},
	}
	for _, tt := range tests {
		t.Run(tt.version+"_"+tt.constraint, func(t *testing.T) {
			v, _ := Parse(tt.version)
			got := Satisfies(v, tt.constraint)
			if got != tt.want {
				t.Errorf("Satisfies(%s, %q) = %v, want %v", tt.version, tt.constraint, got, tt.want)
			}
		})
	}
}

func TestSatisfies_Tilde(t *testing.T) {
	tests := []struct {
		version    string
		constraint string
		want       bool
	}{
		// ~8.1 means >=8.1.0, <8.2.0
		{"8.1.0", "~8.1", true},
		{"8.1.5", "~8.1", true},
		{"8.2.0", "~8.1", false},
		{"8.0.0", "~8.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.version+"_"+tt.constraint, func(t *testing.T) {
			v, _ := Parse(tt.version)
			got := Satisfies(v, tt.constraint)
			if got != tt.want {
				t.Errorf("Satisfies(%s, %q) = %v, want %v", tt.version, tt.constraint, got, tt.want)
			}
		})
	}
}

func TestSatisfies_Or(t *testing.T) {
	tests := []struct {
		version    string
		constraint string
		want       bool
	}{
		// ^10.3 || ^11.0
		{"10.3.0", "^10.3 || ^11.0", true},
		{"11.0.0", "^10.3 || ^11.0", true},
		{"9.0.0", "^10.3 || ^11.0", false},
		{"12.0.0", "^10.3 || ^11.0", false},
		// >=10.0 || ^11
		{"10.0.0", ">=10.0 || ^11", true},
		{"11.5.0", ">=10.0 || ^11", true},
		{"9.0.0", ">=10.0 || ^11", false},
	}
	for _, tt := range tests {
		t.Run(tt.version+"_"+tt.constraint, func(t *testing.T) {
			v, _ := Parse(tt.version)
			got := Satisfies(v, tt.constraint)
			if got != tt.want {
				t.Errorf("Satisfies(%s, %q) = %v, want %v", tt.version, tt.constraint, got, tt.want)
			}
		})
	}
}

func TestSatisfies_InvalidConstraint(t *testing.T) {
	v, _ := Parse("1.0.0")
	// Invalid constraint should return false, not panic.
	if Satisfies(v, "not-a-constraint") {
		t.Error("Satisfies with invalid constraint should return false")
	}
}
