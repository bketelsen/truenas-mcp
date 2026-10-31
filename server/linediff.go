package server

import (
	"fmt"
	"strings"
)

// diffContext is the number of unchanged lines kept on each side of a change in
// each hunk of a unified diff.
const diffContext = 3

// maxDiffCells bounds the dynamic-programming table of the LCS search. Above it
// the changed middle of the input is reported as a full replacement, which is
// still a correct diff: only its shape is coarser. A compose file is far below
// this bound in practice.
const maxDiffCells = 4_000_000

// diffOp is one line of a diff together with its role.
type diffOp struct {
	kind byte // ' ' unchanged, '-' only in the old text, '+' only in the new text
	text string
}

// splitDiffLines splits a text into lines. A trailing newline ends the last
// line instead of adding an empty one.
func splitDiffLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if last := len(lines) - 1; lines[last] == "" {
		lines = lines[:last]
	}
	return lines
}

// lineDiffOps returns the operations that turn before into after: the common
// prefix and suffix are copied, the middle comes from a longest-common-
// subsequence search.
func lineDiffOps(before, after []string) []diffOp {
	prefix := 0
	for prefix < len(before) && prefix < len(after) && before[prefix] == after[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(before)-prefix && suffix < len(after)-prefix &&
		before[len(before)-1-suffix] == after[len(after)-1-suffix] {
		suffix++
	}

	ops := make([]diffOp, 0, len(before)+len(after))
	for _, line := range before[:prefix] {
		ops = append(ops, diffOp{' ', line})
	}
	ops = append(ops, middleDiffOps(before[prefix:len(before)-suffix], after[prefix:len(after)-suffix])...)
	for _, line := range before[len(before)-suffix:] {
		ops = append(ops, diffOp{' ', line})
	}
	return ops
}

// middleDiffOps diffs two texts that share no common prefix or suffix.
func middleDiffOps(before, after []string) []diffOp {
	ops := make([]diffOp, 0, len(before)+len(after))
	if len(before) == 0 || len(after) == 0 || len(before)*len(after) > maxDiffCells {
		for _, line := range before {
			ops = append(ops, diffOp{'-', line})
		}
		for _, line := range after {
			ops = append(ops, diffOp{'+', line})
		}
		return ops
	}

	// table[i][j] holds the length of the longest common subsequence of
	// before[i:] and after[j:].
	table := make([][]int, len(before)+1)
	for i := range table {
		table[i] = make([]int, len(after)+1)
	}
	for i := len(before) - 1; i >= 0; i-- {
		for j := len(after) - 1; j >= 0; j-- {
			if before[i] == after[j] {
				table[i][j] = table[i+1][j+1] + 1
				continue
			}
			table[i][j] = max(table[i+1][j], table[i][j+1])
		}
	}

	i, j := 0, 0
	for i < len(before) && j < len(after) {
		switch {
		case before[i] == after[j]:
			ops = append(ops, diffOp{' ', before[i]})
			i++
			j++
		case table[i+1][j] >= table[i][j+1]:
			ops = append(ops, diffOp{'-', before[i]})
			i++
		default:
			ops = append(ops, diffOp{'+', after[j]})
			j++
		}
	}
	for ; i < len(before); i++ {
		ops = append(ops, diffOp{'-', before[i]})
	}
	for ; j < len(after); j++ {
		ops = append(ops, diffOp{'+', after[j]})
	}
	return ops
}

// unifiedDiff renders a unified diff from the before text to the after text,
// with 3 context lines per hunk. The result is empty when the texts are equal.
func unifiedDiff(oldLabel, newLabel, before, after string) string {
	ops := lineDiffOps(splitDiffLines(before), splitDiffLines(after))
	if !hasChange(ops) {
		return ""
	}

	// oldNo[i] and newNo[i] are the line numbers that the line ops[i] refers
	// to in each text.
	oldNo := make([]int, len(ops)+1)
	newNo := make([]int, len(ops)+1)
	oldLine, newLine := 1, 1
	for i, op := range ops {
		oldNo[i], newNo[i] = oldLine, newLine
		if op.kind != '+' {
			oldLine++
		}
		if op.kind != '-' {
			newLine++
		}
	}
	oldNo[len(ops)], newNo[len(ops)] = oldLine, newLine

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n+++ %s\n", oldLabel, newLabel)

	for i := 0; i < len(ops); {
		for i < len(ops) && ops[i].kind == ' ' {
			i++
		}
		if i == len(ops) {
			break
		}

		start := max(i-diffContext, 0)
		end := i
		for j := i + 1; j < len(ops); j++ {
			if ops[j].kind == ' ' {
				continue
			}
			// Changes closer than twice the context belong to one hunk.
			if j-end-1 > 2*diffContext {
				break
			}
			end = j
		}
		stop := min(end+1+diffContext, len(ops))

		oldCount, newCount := 0, 0
		for _, op := range ops[start:stop] {
			if op.kind != '+' {
				oldCount++
			}
			if op.kind != '-' {
				newCount++
			}
		}
		oldStart, newStart := oldNo[start], newNo[start]
		if oldCount == 0 {
			oldStart--
		}
		if newCount == 0 {
			newStart--
		}

		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount)
		for _, op := range ops[start:stop] {
			b.WriteByte(op.kind)
			b.WriteString(op.text)
			b.WriteByte('\n')
		}
		i = stop
	}
	return b.String()
}

// hasChange reports whether the operations contain an inserted or deleted line.
func hasChange(ops []diffOp) bool {
	for _, op := range ops {
		if op.kind != ' ' {
			return true
		}
	}
	return false
}
