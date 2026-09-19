//go:build darwin

package main

import (
	"fmt"
	"strings"

	"golang.org/x/sys/unix"
)

// Spotlight reads Finder tags from this xattr, and matches them on a plain
// text query, so one shared tag makes the whole set findable by typing it.
const bundleTagAttribute = "com.apple.metadata:_kMDItemUserTags"

// bundleTagDocument is the xattr body for one tag, empty when there is none to
// write, so a write and a drift check cannot disagree on the bytes.
func bundleTagDocument(tag string) string {
	if strings.TrimSpace(tag) == "" {
		return ""
	}
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" ` +
		`"http://www.apple.com/DTDs/PropertyList-1.0.dtd">` +
		`<plist version="1.0"><array><string>` + xmlEscape(tag) +
		`</string></array></plist>`
}

// tagBundle marks one .app so `acompose` in Spotlight surfaces every role at
// once. Finder writes a binary plist here; the XML form indexes identically.
func tagBundle(path, tag string) error {
	document := bundleTagDocument(tag)
	if document == "" {
		return nil
	}
	if err := unix.Setxattr(path, bundleTagAttribute, []byte(document), 0); err != nil {
		return fmt.Errorf("tag %q: %w", path, err)
	}
	return nil
}

// installedBundleTag reads back what tagBundle wrote, empty when untagged.
func installedBundleTag(path string) string {
	size, err := unix.Getxattr(path, bundleTagAttribute, nil)
	if err != nil || size <= 0 {
		return ""
	}
	buffer := make([]byte, size)
	if _, err := unix.Getxattr(path, bundleTagAttribute, buffer); err != nil {
		return ""
	}
	return string(buffer)
}
