package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

type sqlLogic struct {
	cases        int
	coalesces    int
	literalLists int
	intervals    int
}

var (
	sqlCasePattern        = regexp.MustCompile(`\bCASE\b`)
	sqlCoalescePattern    = regexp.MustCompile(`\bCOALESCE\s*\(`)
	sqlLiteralListPattern = regexp.MustCompile(`\bIN\s*\(\s*'`)
	sqlIntervalPattern    = regexp.MustCompile(`\bINTERVAL\s*'|'::INTERVAL\b`)
)

var sqlLogicBaseline = map[string]sqlLogic{
	"AggregateCostEventsWindow":       {coalesces: 4},
	"AggregateSessionSummariesWindow": {coalesces: 4},
	"CountActiveRuns":                 {literalLists: 1},
	"CountRunsNeedingCleanup":         {literalLists: 1},
	"CountTraceMaskingInWindow":       {cases: 1, coalesces: 1},
	"GetLiveSkillListingFacts":        {coalesces: 4},
	"GetTraceEventText":               {coalesces: 1},
	"GetTraceStreamHealth":            {coalesces: 1},
	"ListActiveRuns":                  {literalLists: 1},
	"ListHybridSearchCandidates":      {coalesces: 3},
	"ListLiveSkillsForIndex":          {coalesces: 1},
	"ListRunsNeedingCleanup":          {literalLists: 1},
	"ListSkills":                      {coalesces: 1},
	"ListSkillsPastDeletionGrace":     {coalesces: 1},
	"ListTraceGeneralFacts":           {coalesces: 16},
	"ListWorkspacePurgeCandidates":    {coalesces: 1},
	"MarkAccountPurgeStarted":         {coalesces: 1},
	"MarkDatasetObjectLost":           {coalesces: 1},
	"MarkDatasetPurged":               {coalesces: 1},
	"NextTraceSeq":                    {coalesces: 1},
	"PublicSearchSkills":              {cases: 1},
	"RecordOutboxDeliveryFailure":     {cases: 1},
	"RequestAccountDeletion":          {cases: 2, coalesces: 1},
	"SetRunCleanupStatus":             {coalesces: 1},
	"SumCreditBalances":               {coalesces: 1},
	"SumDatasetUsage":                 {coalesces: 1},
	"UpsertSearchDocumentEnriched":    {cases: 1},
}

func sqlLogicOf(body string) sqlLogic {
	sql := strings.ToUpper(sqlCommentPattern.ReplaceAllString(body, " "))
	return sqlLogic{
		cases:        len(sqlCasePattern.FindAllString(sql, -1)),
		coalesces:    len(sqlCoalescePattern.FindAllString(sql, -1)),
		literalLists: len(sqlLiteralListPattern.FindAllString(sql, -1)),
		intervals:    len(sqlIntervalPattern.FindAllString(sql, -1)),
	}
}

func (l sqlLogic) exceeds(baseline sqlLogic) bool {
	return l.cases > baseline.cases || l.coalesces > baseline.coalesces ||
		l.literalLists > baseline.literalLists || l.intervals > baseline.intervals
}

func (l sqlLogic) String() string {
	return fmt.Sprintf("CASE %d, COALESCE %d, literal IN lists %d, interval literals %d",
		l.cases, l.coalesces, l.literalLists, l.intervals)
}

func sqlLogicProblems(root string) []string {
	queries, err := loadSQLQueries(filepath.Join(root, "db", "queries"))
	if err != nil {
		return []string{fmt.Sprintf("db/queries: %v", err)}
	}
	return sqlLogicRatchet(queries, sqlLogicBaseline)
}

func sqlLogicRatchet(queries map[string]sqlQuery, baselines map[string]sqlLogic) []string {
	var problems []string
	for _, name := range sortedKeys(queries) {
		got, baseline := queries[name].logic, baselines[name]
		switch {
		case got.exceeds(baseline):
			problems = append(problems, fmt.Sprintf(
				"db/queries/%s: %s has %s, more than its baseline (%s); decide the value in Go and pass it as a parameter",
				queries[name].file, name, got, baseline))
		case got != baseline:
			problems = append(problems, fmt.Sprintf(
				"tools/devctl/sql_logic.go: %s now has %s; lower its baseline to match so the gain cannot be spent again",
				name, got))
		}
	}
	for _, name := range sortedKeys(baselines) {
		if _, exists := queries[name]; !exists {
			problems = append(problems, fmt.Sprintf(
				"tools/devctl/sql_logic.go: %s is not a query in db/queries; drop its baseline", name))
		}
	}
	return problems
}
