package adapter

import (
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Major int
	Minor int
	Patch int
}

func ParseVersion(raw string) (Version, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "v")
	if raw == "" {
		return Version{}, fmt.Errorf("empty version")
	}

	parts := strings.SplitN(raw, "+", 2)
	raw = parts[0]
	parts = strings.SplitN(raw, "-", 2)
	raw = parts[0]

	nums := strings.Split(raw, ".")
	if len(nums) > 3 {
		return Version{}, fmt.Errorf("invalid version %q", raw)
	}

	parse := func(s string) (int, error) {
		if s == "" {
			return 0, nil
		}
		return strconv.Atoi(s)
	}

	var version Version
	var err error
	if version.Major, err = parse(nums[0]); err != nil {
		return Version{}, fmt.Errorf("invalid version %q: %w", raw, err)
	}
	if len(nums) > 1 {
		if version.Minor, err = parse(nums[1]); err != nil {
			return Version{}, fmt.Errorf("invalid version %q: %w", raw, err)
		}
	}
	if len(nums) > 2 {
		if version.Patch, err = parse(nums[2]); err != nil {
			return Version{}, fmt.Errorf("invalid version %q: %w", raw, err)
		}
	}
	return version, nil
}

func (v Version) Compare(other Version) int {
	switch {
	case v.Major < other.Major:
		return -1
	case v.Major > other.Major:
		return 1
	case v.Minor < other.Minor:
		return -1
	case v.Minor > other.Minor:
		return 1
	case v.Patch < other.Patch:
		return -1
	case v.Patch > other.Patch:
		return 1
	default:
		return 0
	}
}

type versionClause struct {
	op      string
	version Version
}

type VersionRange struct {
	Raw     string
	clauses []versionClause
}

func ParseVersionRange(raw string) (VersionRange, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return VersionRange{}, fmt.Errorf("empty version range")
	}

	tokens := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' '
	})

	clauses := make([]versionClause, 0, len(tokens))
	for _, token := range tokens {
		if token == "" {
			continue
		}
		clause, err := parseVersionClause(token)
		if err != nil {
			return VersionRange{}, err
		}
		clauses = append(clauses, clause)
	}

	if len(clauses) == 0 {
		return VersionRange{}, fmt.Errorf("empty version range")
	}

	return VersionRange{Raw: raw, clauses: clauses}, nil
}

func parseVersionClause(token string) (versionClause, error) {
	ops := []string{">=", "<=", "==", "!=", ">", "<", "="}
	for _, op := range ops {
		if strings.HasPrefix(token, op) {
			ver, err := ParseVersion(strings.TrimSpace(strings.TrimPrefix(token, op)))
			if err != nil {
				return versionClause{}, err
			}
			return versionClause{op: op, version: ver}, nil
		}
	}

	ver, err := ParseVersion(token)
	if err != nil {
		return versionClause{}, err
	}
	return versionClause{op: "=", version: ver}, nil
}

func (r VersionRange) Matches(rawVersion string) (bool, error) {
	if len(r.clauses) == 0 {
		return false, fmt.Errorf("version range %q is not parsed", r.Raw)
	}

	version, err := ParseVersion(rawVersion)
	if err != nil {
		return false, err
	}

	for _, clause := range r.clauses {
		cmp := version.Compare(clause.version)
		switch clause.op {
		case "=", "==":
			if cmp != 0 {
				return false, nil
			}
		case "!=":
			if cmp == 0 {
				return false, nil
			}
		case ">":
			if cmp <= 0 {
				return false, nil
			}
		case ">=":
			if cmp < 0 {
				return false, nil
			}
		case "<":
			if cmp >= 0 {
				return false, nil
			}
		case "<=":
			if cmp > 0 {
				return false, nil
			}
		default:
			return false, fmt.Errorf("unsupported version operator %q", clause.op)
		}
	}

	return true, nil
}
