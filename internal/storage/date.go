package storage

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const DateLayout = "2006-01-02"

var dateLayouts = []string{
	DateLayout,
	"2006/01/02",
	"02/01/2006",
	"02-01-2006",
	time.RFC3339,
}

func Today() string {
	return time.Now().Format(DateLayout)
}

func NormalizeDate(input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", nil
	}

	for _, layout := range dateLayouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t.Format(DateLayout), nil
		}
	}

	return "", fmt.Errorf("invalid date %q, expected YYYY-MM-DD", input)
}

func CurrentMonthRange() (string, string) {
	start, end, _, _ := MonthRange(Today())
	return start, end
}

func CurrentStatementPeriodRange() (string, string) {
	start, end, _, _ := StatementPeriodRange(Today())
	return start, end
}

func MonthRange(input string) (string, string, string, error) {
	date, err := NormalizeDate(input)
	if err != nil {
		return "", "", "", err
	}
	if date == "" {
		date = Today()
	}

	t, err := time.Parse(DateLayout, date)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid date %q, expected YYYY-MM-DD", input)
	}

	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	end := start.AddDate(0, 1, 0)
	return start.Format(DateLayout), end.Format(DateLayout), start.Format("2006-01"), nil
}

func StatementPeriodRange(input string) (string, string, string, error) {
	date, err := NormalizeDate(input)
	if err != nil {
		return "", "", "", err
	}
	if date == "" {
		date = Today()
	}

	t, err := time.Parse(DateLayout, date)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid date %q, expected YYYY-MM-DD", input)
	}

	start := time.Date(t.Year(), t.Month(), 21, 0, 0, 0, 0, t.Location())
	if t.Day() < 21 {
		start = start.AddDate(0, -1, 0)
	}
	end := start.AddDate(0, 1, 0)
	return start.Format(DateLayout), end.Format(DateLayout), fmt.Sprintf("%s to %s", start.Format(DateLayout), end.AddDate(0, 0, -1).Format(DateLayout)), nil
}

func StatementPeriodID(input string) (int64, error) {
	start, _, _, err := StatementPeriodRange(input)
	if err != nil {
		return 0, err
	}

	t, err := time.Parse(DateLayout, start)
	if err != nil {
		return 0, err
	}

	id, err := strconv.ParseInt(t.Format("0601"), 10, 64)
	if err != nil {
		return 0, err
	}
	return id, nil
}
