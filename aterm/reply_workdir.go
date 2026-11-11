package main

import (
	"fmt"
	"regexp"
	"strings"
)

var shadowSessionID = regexp.MustCompile(`^[0-9A-Za-z]{2,16}$`)

// shadowID is the native shadow the seat ran in, from its argv. A dead seat has no pid
// for withLiveCwd, yet its harness worked in <session>/projects.
func (entry ledgerEntry) shadowID() string {
	for i, arg := range entry.Argv {
		if arg != "_native-shadow" {
			continue
		}
		for j := i + 1; j+1 < len(entry.Argv) && entry.Argv[j] != "--"; j++ {
			if entry.Argv[j] == "--session-id" && shadowSessionID.MatchString(entry.Argv[j+1]) {
				return entry.Argv[j+1]
			}
		}
	}
	return ""
}

// workdirKey groups seats that could share a harness session store: a shadow is its own.
func (entry ledgerEntry) workdirKey() string {
	if id := entry.shadowID(); id != "" {
		return "shadow:" + id
	}
	return entry.Cwd
}

// inWorkdir reports whether a directory a harness recorded is this seat's.
func (entry ledgerEntry) inWorkdir(path string) bool {
	if path == entry.Cwd {
		return true
	}
	id := entry.shadowID()
	return id != "" && (strings.HasSuffix(path, "/"+id+"/projects") || strings.Contains(path, "/"+id+"/projects/"))
}

// dirClause is the SQL condition for a column holding the seat's directory.
func (entry ledgerEntry) dirClause(column string) (string, error) {
	dir, err := sqlQuote(entry.Cwd)
	if err != nil {
		return "", err
	}
	clause := column + " = " + dir
	if id := entry.shadowID(); id != "" {
		clause += fmt.Sprintf(" OR %s LIKE '%%/%s/projects' OR %s LIKE '%%/%s/projects/%%'", column, id, column, id)
	}
	return "(" + clause + ")", nil
}
