/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package version

import (
	"cmp"
	"fmt"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
)

const (
	// UnknownVersion is reported for failed version detection.
	UnknownVersion = "0.0.0-unknown"
	// DevelVersion is what we get from debug/build info when building
	// plugins within the NRI repository.
	DevelVersion = "(devel)"
	// nriModulePath is the module we look for to discover the NRI version.
	nriModulePath = "github.com/containerd/nri"
)

// version represents a struct type that holds relevant data
// that constitute a Semantic Version
type version struct {
	major int
	minor int
	patch int
	pre   string
}

func (v version) String() string {
	if v.pre != "" {
		return fmt.Sprintf("v%d.%d.%d-%s", v.major, v.minor, v.patch, v.pre)
	}
	return fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch)
}

// compareVersion parses two semver strings into the "version" struct type and compares them.
// NOTE: It ignores the build metadata when making the comparison
func compareVersion(a, b string) int {
	aVer, errA := parseVersion(a)
	bVer, errB := parseVersion(b)
	if errA != nil || errB != nil {
		return cmp.Compare(a, b)
	}

	if c := cmp.Compare(aVer.major, bVer.major); c != 0 {
		return c
	}
	if c := cmp.Compare(aVer.minor, bVer.minor); c != 0 {
		return c
	}
	if c := cmp.Compare(aVer.patch, bVer.patch); c != 0 {
		return c
	}
	if aVer.pre == "" || bVer.pre == "" {
		if aVer.pre != "" {
			return -1
		} else if bVer.pre != "" {
			return 1
		}
	}
	return cmp.Compare(aVer.pre, bVer.pre)
}

func parseVersion(s string) (version, error) {
	major, rest, _ := strings.Cut(s, ".")
	minor, patch, _ := strings.Cut(rest, ".")

	var pre string
	if len(patch) > 0 {
		patch, pre, _ = strings.Cut(patch, "-")
		if pre != "" {
			pre, _, _ = strings.Cut(pre, "+")
		} else {
			patch, _, _ = strings.Cut(patch, "+")
		}
	}

	var v version
	var err error
	if len(major) > 0 {
		if v.major, err = strconv.Atoi(strings.TrimPrefix(major, "v")); err != nil {
			return version{}, err
		}
	}
	if len(minor) > 0 {
		if v.minor, err = strconv.Atoi(minor); err != nil {
			return version{}, err
		}
	}
	if len(patch) > 0 {
		if v.patch, err = strconv.Atoi(patch); err != nil {
			return version{}, err
		}
	}
	v.pre = pre

	return v, nil
}

// GetFromBuildInfo returns the locally used NRI version. This
// is taken either from the debug/build info provided by the
// golang runtime, or for plugins hosted in the NRI repository
// from a git-described version generated at build time.
func GetFromBuildInfo() string {
	version := UnknownVersion

	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, mod := range bi.Deps {
			if mod.Path != nriModulePath {
				continue
			}

			if mod.Replace != nil && mod.Replace.Version != DevelVersion {
				version = mod.Replace.Version
			} else {
				version = mod.Version
			}
		}
	}

	if version == DevelVersion {
		return fallbackVersion()
	}

	return version
}

// majorMinorPatch returns the major.minor.patch prefix of the semantic version v.
func majorMinorPatch(v version) string {
	return fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch)
}

// FindClosestMatch returns the largest version smaller or equal to a given one.
// "" is returned if no such version if found.
func FindClosestMatch(v string, versions []string) string {
	// Note: A git-described version suffix (-N-gSHA1[.*])) is not semantically
	// semver-correct as semver considers it a prerelease identifier. Therefore
	// semver for instance considers v2.2.0-225-ge9dc15b7a.m < v2.2.0, which is
	// obviously not the case. In lack of a better choice, we strip any such
	// suffix from v before comparison.
	v = stripGitSuffix(v)

	slices.SortFunc(versions, compareVersion)

	latest := ""
	for _, ver := range versions {
		if compareVersion(ver, v) > 0 {
			break
		}
		latest = ver
	}
	return latest
}

// stripGitSuffix strips any git described suffix from a version string.
// We expect a valid git suffix to be of the form "-N-gSHA1[.m], where
// N is an decimal integer and SHA1 is a hexadecimal integer.
func stripGitSuffix(version string) string {
	pv, _ := parseVersion(version)
	mmp := majorMinorPatch(pv)
	if pv.String() != version {
		return version
	}

	pre := pv.pre
	if len(pre) == 0 {
		return version
	}

	commits, gsha1, ok := strings.Cut(pre, "-")
	if !ok || len(gsha1) == 0 || gsha1[0] != 'g' {
		return version
	}
	if _, err := strconv.ParseInt(commits, 10, 64); err != nil {
		return version
	}

	sha1, _, _ := strings.Cut(gsha1[1:], ".")
	if _, err := strconv.ParseInt(sha1, 16, 64); err != nil {
		return version
	}

	return mmp
}
