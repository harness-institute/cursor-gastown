//go:build darwin

package util

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// GetDiskSpace returns filesystem space information for the given path.
// On APFS volumes it queries diskutil to include purgeable space (iCloud
// local copies, Time Machine snapshots, etc.) in the available bytes,
// preventing false-positive "disk exhausted" blocks on macOS.
func GetDiskSpace(path string) (*DiskSpaceInfo, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, fmt.Errorf("statfs %s: %w", path, err)
	}

	bsize := uint64(stat.Bsize)
	total := stat.Blocks * bsize
	free := uint64(stat.Bavail) * bsize //nolint:unconvert
	used := total - (stat.Bfree * bsize)

	// On APFS, APFSContainerFree from diskutil includes purgeable space that
	// macOS reclaims automatically under pressure — statfs Bavail omits it.
	if int8SliceToString(stat.Fstypename[:]) == "apfs" {
		mountPoint := int8SliceToString(stat.Mntonname[:])
		if containerFree, containerSize, err := apfsContainerSpaceFn(mountPoint); err == nil {
			free = containerFree
			if containerSize > 0 {
				total = containerSize
			}
			if free <= total {
				used = total - free
			} else {
				used = 0
			}
		}
	}

	var usedPct float64
	if total > 0 {
		usedPct = float64(used) / float64(total) * 100
	}

	return &DiskSpaceInfo{
		AvailableBytes: free,
		TotalBytes:     total,
		UsedBytes:      used,
		UsedPercent:    usedPct,
	}, nil
}

// int8SliceToString converts the signed-byte fields used by darwin's
// syscall.Statfs_t (e.g. Fstypename, Mntonname) into a Go string. The
// standard `string(bytes)` idiom doesn't apply because the underlying
// type is []int8, not []byte (signedness quirk of darwin syscalls).
func int8SliceToString(b []int8) string {
	buf := make([]byte, len(b))
	for i, v := range b {
		if v == 0 {
			return string(buf[:i])
		}
		buf[i] = byte(v)
	}
	return string(buf)
}

// apfsContainerSpaceFn is the function used to query APFS container space.
// It is a variable so tests can replace it with a stub.
var apfsContainerSpaceFn = apfsContainerSpace

// apfsContainerSpace returns the APFS container free and total bytes by
// calling diskutil. Returns an error if diskutil is unavailable or the output
// lacks the expected keys.
func apfsContainerSpace(mountPoint string) (free, total uint64, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "diskutil", "info", "-plist", mountPoint).Output()
	if err != nil {
		return 0, 0, fmt.Errorf("diskutil info: %w", err)
	}

	free, ok := parsePlistUint64(out, "APFSContainerFree")
	if !ok || free == 0 {
		return 0, 0, fmt.Errorf("APFSContainerFree not found in diskutil output")
	}
	total, _ = parsePlistUint64(out, "APFSContainerSize")
	return free, total, nil
}

// parsePlistUint64 extracts a uint64 integer for the given key from Apple plist XML.
func parsePlistUint64(data []byte, key string) (uint64, bool) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	waitForValue := false
	for {
		tok, err := dec.Token()
		if err != nil {
			// any xml decode error (incl. io.EOF) → treat as "key not found";
			// caller falls back to statfs values, which is the safe default.
			return 0, false
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "key":
				var k string
				if err := dec.DecodeElement(&k, &t); err == nil {
					waitForValue = k == key
				} else {
					waitForValue = false
				}
			case "integer":
				if waitForValue {
					var s string
					if err := dec.DecodeElement(&s, &t); err == nil {
						if v, err := strconv.ParseUint(s, 10, 64); err == nil {
							return v, true
						}
					}
					return 0, false
				}
				waitForValue = false
			default:
				waitForValue = false
			}
		}
	}
}
