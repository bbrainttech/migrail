package ir

import (
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Major int
	Minor int
}

func ParseVersion(text string) (Version, error) {
	majorText, minorText, hasMinor := strings.Cut(strings.TrimSpace(text), ".")

	major, err := strconv.Atoi(majorText)
	if err != nil || major <= 0 {
		return Version{}, fmt.Errorf("invalid version %q: expected a number such as 16 or 16.4", text)
	}

	if !hasMinor {
		return Version{Major: major}, nil
	}

	minor, err := strconv.Atoi(minorText)
	if err != nil || minor < 0 {
		return Version{}, fmt.Errorf("invalid version %q: expected a number such as 16 or 16.4", text)
	}

	return Version{Major: major, Minor: minor}, nil
}

func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		return v.Major - other.Major
	}

	return v.Minor - other.Minor
}

func (v Version) String() string {
	if v.Minor == 0 {
		return strconv.Itoa(v.Major)
	}

	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}
