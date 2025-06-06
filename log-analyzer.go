package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// LogEntry represents a parsed log entry
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
	Source    string
	Raw       string
}

// LogStats holds statistics about the log file
type LogStats struct {
	TotalLines   int
	ErrorCount   int
	WarnCount    int
	InfoCount    int
	DebugCount   int
	TimeRange    string
	TopSources   map[string]int
	TopErrors    map[string]int
}

// LogAnalyzer handles log parsing and analysis
type LogAnalyzer struct {
	entries   []LogEntry
	patterns  map[string]*regexp.Regexp
	filters   Filters
}

// Filters contains filtering options
type Filters struct {
	Level     string
	StartTime *time.Time
	EndTime   *time.Time
	Source    string
	Keyword   string
}

// Common log patterns
var logPatterns = map[string]string{
	"apache":   `^(\S+) \S+ \S+ \[([^\]]+)\] "([^"]*)" (\d+) (\d+)`,
	"nginx":    `^(\S+) - - \[([^\]]+)\] "([^"]*)" (\d+) (\d+) "([^"]*)" "([^"]*)"`,
	"syslog":   `^(\w+\s+\d+\s+\d+:\d+:\d+) (\S+) ([^:]+): (.*)`,
	"generic":  `^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})\s+\[(\w+)\]\s+(.*)`,
	"json":     `^\{.*\}$`,
}

func main() {
	var (
		filename   = flag.String("f", "", "Log file to analyze")
		format     = flag.String("format", "auto", "Log format (apache, nginx, syslog, generic, json, auto)")
		level      = flag.String("level", "", "Filter by log level (ERROR, WARN, INFO, DEBUG)")
		source     = flag.String("source", "", "Filter by source/component")
		keyword    = flag.String("keyword", "", "Filter by keyword in message")
		startTime  = flag.String("start", "", "Start time filter (YYYY-MM-DD HH:MM:SS)")
		endTime    = flag.String("end", "", "End time filter (YYYY-MM-DD HH:MM:SS)")
		stats      = flag.Bool("stats", false, "Show statistics")
		tail       = flag.Int("tail", 0, "Show last N lines")
		head       = flag.Int("head", 0, "Show first N lines")
		follow     = flag.Bool("follow", false, "Follow log file (like tail -f)")
		output     = flag.String("output", "", "Output format (json, csv)")
		verbose    = flag.Bool("v", false, "Verbose output")
	)
	flag.Parse()

	if *filename == "" {
		fmt.Println("Usage: loganalyzer -f <logfile> [options]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	analyzer := NewLogAnalyzer()

	// Parse time filters
	filters := Filters{
		Level:   strings.ToUpper(*level),
		Source:  *source,
		Keyword: *keyword,
	}

	if *startTime != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", *startTime); err == nil {
			filters.StartTime = &t
		} else {
			log.Fatalf("Invalid start time format: %v", err)
		}
	}

	if *endTime != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", *endTime); err == nil {
			filters.EndTime = &t
		} else {
			log.Fatalf("Invalid end time format: %v", err)
		}
	}

	analyzer.filters = filters

	if *follow {
		analyzer.followFile(*filename, *format, *verbose)
	} else {
		if err := analyzer.parseFile(*filename, *format); err != nil {
			log.Fatalf("Error parsing file: %v", err)
		}

		filteredEntries := analyzer.filterEntries()

		if *stats {
			analyzer.showStats()
			return
		}

		if *head > 0 {
			filteredEntries = analyzer.getHead(filteredEntries, *head)
		} else if *tail > 0 {
			filteredEntries = analyzer.getTail(filteredEntries, *tail)
		}

		analyzer.outputEntries(filteredEntries, *output, *verbose)
	}
}

func NewLogAnalyzer() *LogAnalyzer {
	analyzer := &LogAnalyzer{
		entries:  make([]LogEntry, 0),
		patterns: make(map[string]*regexp.Regexp),
	}

	// Compile regex patterns
	for name, pattern := range logPatterns {
		if regex, err := regexp.Compile(pattern); err == nil {
			analyzer.patterns[name] = regex
		}
	}

	return analyzer
}

func (la *LogAnalyzer) parseFile(filename, format string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		
		if entry := la.parseLine(line, format); entry != nil {
			la.entries = append(la.entries, *entry)
		}
	}

	return scanner.Err()
}

func (la *LogAnalyzer) parseLine(line, format string) *LogEntry {
	entry := &LogEntry{Raw: line}

	if format == "json" || (format == "auto" && strings.HasPrefix(strings.TrimSpace(line), "{")) {
		return la.parseJSON(line)
	}

	// Try different patterns based on format
	patterns := []string{format}
	if format == "auto" {
		patterns = []string{"generic", "syslog", "apache", "nginx"}
	}

	for _, patternName := range patterns {
		if regex, exists := la.patterns[patternName]; exists {
			if matches := regex.FindStringSubmatch(line); matches != nil {
				return la.parseWithPattern(line, patternName, matches)
			}
		}
	}

	// Fallback: treat as plain text with timestamp detection
	return la.parseGeneric(line)
}

func (la *LogAnalyzer) parseJSON(line string) *LogEntry {
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(line), &jsonData); err != nil {
		return &LogEntry{Raw: line, Message: line}
	}

	entry := &LogEntry{Raw: line}

	// Try to extract common fields
	if timestamp, ok := jsonData["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
			entry.Timestamp = t
		}
	}

	if level, ok := jsonData["level"].(string); ok {
		entry.Level = strings.ToUpper(level)
	}

	if message, ok := jsonData["message"].(string); ok {
		entry.Message = message
	} else if msg, ok := jsonData["msg"].(string); ok {
		entry.Message = msg
	}

	if source, ok := jsonData["source"].(string); ok {
		entry.Source = source
	} else if component, ok := jsonData["component"].(string); ok {
		entry.Source = component
	}

	return entry
}

func (la *LogAnalyzer) parseWithPattern(line, patternName string, matches []string) *LogEntry {
	entry := &LogEntry{Raw: line}

	switch patternName {
	case "generic":
		if len(matches) >= 4 {
			if t, err := time.Parse("2006-01-02 15:04:05", matches[1]); err == nil {
				entry.Timestamp = t
			}
			entry.Level = strings.ToUpper(matches[2])
			entry.Message = matches[3]
		}
	case "syslog":
		if len(matches) >= 5 {
			if t, err := time.Parse("Jan 2 15:04:05", matches[1]); err == nil {
				// Add current year since syslog doesn't include it
				entry.Timestamp = t.AddDate(time.Now().Year(), 0, 0)
			}
			entry.Source = matches[2]
			entry.Message = matches[4]
			entry.Level = la.inferLogLevel(matches[4])
		}
	case "apache", "nginx":
		if len(matches) >= 4 {
			entry.Source = matches[1]
			if t, err := time.Parse("02/Jan/2006:15:04:05 -0700", matches[2]); err == nil {
				entry.Timestamp = t
			}
			entry.Message = matches[3]
			
			// Infer level from HTTP status code
			if len(matches) >= 5 {
				if status, err := strconv.Atoi(matches[4]); err == nil {
					if status >= 500 {
						entry.Level = "ERROR"
					} else if status >= 400 {
						entry.Level = "WARN"
					} else {
						entry.Level = "INFO"
					}
				}
			}
		}
	}

	return entry
}

func (la *LogAnalyzer) parseGeneric(line string) *LogEntry {
	entry := &LogEntry{Raw: line, Message: line}

	// Try to find timestamp at beginning of line
	timePatterns := []string{
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"Jan 2 15:04:05",
		"2006-01-02T15:04:05Z07:00",
	}

	for _, pattern := range timePatterns {
		if len(line) >= len(pattern) {
			if t, err := time.Parse(pattern, line[:len(pattern)]); err == nil {
				entry.Timestamp = t
				if len(line) > len(pattern)+1 {
					entry.Message = strings.TrimSpace(line[len(pattern)+1:])
				}
				break
			}
		}
	}

	// Infer log level from message content
	entry.Level = la.inferLogLevel(entry.Message)

	return entry
}

func (la *LogAnalyzer) inferLogLevel(message string) string {
	message = strings.ToUpper(message)
	
	if strings.Contains(message, "ERROR") || strings.Contains(message, "FATAL") || strings.Contains(message, "CRITICAL") {
		return "ERROR"
	}
	if strings.Contains(message, "WARN") || strings.Contains(message, "WARNING") {
		return "WARN"
	}
	if strings.Contains(message, "DEBUG") || strings.Contains(message, "TRACE") {
		return "DEBUG"
	}
	
	return "INFO"
}

func (la *LogAnalyzer) filterEntries() []LogEntry {
	var filtered []LogEntry

	for _, entry := range la.entries {
		if la.filters.Level != "" && entry.Level != la.filters.Level {
			continue
		}

		if la.filters.Source != "" && !strings.Contains(strings.ToLower(entry.Source), strings.ToLower(la.filters.Source)) {
			continue
		}

		if la.filters.Keyword != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(la.filters.Keyword)) {
			continue
		}

		if la.filters.StartTime != nil && !entry.Timestamp.IsZero() && entry.Timestamp.Before(*la.filters.StartTime) {
			continue
		}

		if la.filters.EndTime != nil && !entry.Timestamp.IsZero() && entry.Timestamp.After(*la.filters.EndTime) {
			continue
		}

		filtered = append(filtered, entry)
	}

	return filtered
}

func (la *LogAnalyzer) getHead(entries []LogEntry, n int) []LogEntry {
	if n >= len(entries) {
		return entries
	}
	return entries[:n]
}

func (la *LogAnalyzer) getTail(entries []LogEntry, n int) []LogEntry {
	if n >= len(entries) {
		return entries
	}
	return entries[len(entries)-n:]
}

func (la *LogAnalyzer) outputEntries(entries []LogEntry, format string, verbose bool) {
	switch format {
	case "json":
		la.outputJSON(entries)
	case "csv":
		la.outputCSV(entries)
	default:
		la.outputText(entries, verbose)
	}
}

func (la *LogAnalyzer) outputText(entries []LogEntry, verbose bool) {
	for _, entry := range entries {
		if verbose {
			fmt.Printf("[%s] [%s] [%s] %s\n", 
				entry.Timestamp.Format("2006-01-02 15:04:05"), 
				entry.Level, 
				entry.Source, 
				entry.Message)
		} else {
			if !entry.Timestamp.IsZero() {
				fmt.Printf("%s ", entry.Timestamp.Format("15:04:05"))
			}
			if entry.Level != "" {
				fmt.Printf("[%s] ", entry.Level)
			}
			fmt.Println(entry.Message)
		}
	}
}

func (la *LogAnalyzer) outputJSON(entries []LogEntry) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(entries)
}

func (la *LogAnalyzer) outputCSV(entries []LogEntry) {
	fmt.Println("Timestamp,Level,Source,Message")
	for _, entry := range entries {
		timestamp := ""
		if !entry.Timestamp.IsZero() {
			timestamp = entry.Timestamp.Format("2006-01-02 15:04:05")
		}
		fmt.Printf("%s,%s,%s,\"%s\"\n", timestamp, entry.Level, entry.Source, 
			strings.ReplaceAll(entry.Message, "\"", "\"\""))
	}
}

func (la *LogAnalyzer) showStats() {
	stats := LogStats{
		TotalLines: len(la.entries),
		TopSources: make(map[string]int),
		TopErrors:  make(map[string]int),
	}

	var earliest, latest time.Time

	for _, entry := range la.entries {
		switch entry.Level {
		case "ERROR":
			stats.ErrorCount++
			stats.TopErrors[entry.Message]++
		case "WARN":
			stats.WarnCount++
		case "INFO":
			stats.InfoCount++
		case "DEBUG":
			stats.DebugCount++
		}

		if entry.Source != "" {
			stats.TopSources[entry.Source]++
		}

		if !entry.Timestamp.IsZero() {
			if earliest.IsZero() || entry.Timestamp.Before(earliest) {
				earliest = entry.Timestamp
			}
			if latest.IsZero() || entry.Timestamp.After(latest) {
				latest = entry.Timestamp
			}
		}
	}

	if !earliest.IsZero() && !latest.IsZero() {
		stats.TimeRange = fmt.Sprintf("%s to %s", 
			earliest.Format("2006-01-02 15:04:05"), 
			latest.Format("2006-01-02 15:04:05"))
	}

	fmt.Println("=== Log Analysis Statistics ===")
	fmt.Printf("Total Lines: %d\n", stats.TotalLines)
	fmt.Printf("Time Range: %s\n", stats.TimeRange)
	fmt.Println()
	fmt.Printf("Log Levels:\n")
	fmt.Printf("  ERROR: %d\n", stats.ErrorCount)
	fmt.Printf("  WARN:  %d\n", stats.WarnCount)
	fmt.Printf("  INFO:  %d\n", stats.InfoCount)
	fmt.Printf("  DEBUG: %d\n", stats.DebugCount)
	fmt.Println()

	if len(stats.TopSources) > 0 {
		fmt.Println("Top Sources:")
		la.printTopMap(stats.TopSources, 5)
		fmt.Println()
	}

	if len(stats.TopErrors) > 0 {
		fmt.Println("Top Errors:")
		la.printTopMap(stats.TopErrors, 5)
	}
}

func (la *LogAnalyzer) printTopMap(m map[string]int, limit int) {
	type kv struct {
		Key   string
		Value int
	}

	var ss []kv
	for k, v := range m {
		ss = append(ss, kv{k, v})
	}

	sort.Slice(ss, func(i, j int) bool {
		return ss[i].Value > ss[j].Value
	})

	for i, kv := range ss {
		if i >= limit {
			break
		}
		fmt.Printf("  %s: %d\n", kv.Key, kv.Value)
	}
}

func (la *LogAnalyzer) followFile(filename, format string, verbose bool) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	defer file.Close()

	// Seek to end of file
	file.Seek(0, 2)

	scanner := bufio.NewScanner(file)
	fmt.Println("Following log file... (Press Ctrl+C to exit)")

	for {
		if scanner.Scan() {
			line := scanner.Text()
			if entry := la.parseLine(line, format); entry != nil {
				// Apply filters
				if la.matchesFilters(*entry) {
					la.outputEntries([]LogEntry{*entry}, "", verbose)
				}
			}
		} else {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (la *LogAnalyzer) matchesFilters(entry LogEntry) bool {
	if la.filters.Level != "" && entry.Level != la.filters.Level {
		return false
	}

	if la.filters.Source != "" && !strings.Contains(strings.ToLower(entry.Source), strings.ToLower(la.filters.Source)) {
		return false
	}

	if la.filters.Keyword != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(la.filters.Keyword)) {
		return false
	}

	if la.filters.StartTime != nil && !entry.Timestamp.IsZero() && entry.Timestamp.Before(*la.filters.StartTime) {
		return false
	}

	if la.filters.EndTime != nil && !entry.Timestamp.IsZero() && entry.Timestamp.After(*la.filters.EndTime) {
		return false
	}

	return true
}