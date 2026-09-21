package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatementDefaultsAndSearch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "budgeter.db")

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := New(&out, &errOut)

	err := app.Run([]string{
		"--db", dbPath,
		"statement", "load",
		"--date", "2026-08-20",
		"--account", "cheque",
		"--balance", "12000.00",
		"--income", "5000.00",
		"--costs", "2000.00",
		"--earnings", "3000.00",
		"--savings", "1000.00",
	})
	if err != nil {
		t.Fatalf("first statement load error = %v", err)
	}
	if !strings.Contains(out.String(), "Loaded statement #2607 for 2026-08-20 (statement period 2026-07-21 to 2026-08-20)") {
		t.Fatalf("statement load output missing period:\n%s", out.String())
	}

	out.Reset()
	err = app.Run([]string{
		"--db", dbPath,
		"statement", "load",
		"--date", "2026-09-21",
		"--balance", "12500.00",
	})
	if err != nil {
		t.Fatalf("second statement load error = %v", err)
	}

	out.Reset()
	err = app.Run([]string{
		"--db", dbPath,
		"statement", "tx", "load",
		"--statement-id", "2609",
		"--date", "2026-09-21",
		"--bucket", "cost",
		"--category", "groceries",
		"--description", "Market",
		"--amount", "-42.10",
	})
	if err != nil {
		t.Fatalf("transaction load error = %v", err)
	}

	out.Reset()
	err = app.Run([]string{
		"--db", dbPath,
		"statement", "tx", "load",
		"--statement-id", "2609",
		"--date", "2026-09-21",
		"--bucket", "income",
		"--category", "miscellaneous",
		"--description", "Payroll",
		"--amount", "5000.00",
	})
	if err != nil {
		t.Fatalf("income transaction load error = %v", err)
	}

	out.Reset()
	err = app.Run([]string{
		"--db", dbPath,
		"statement", "tx", "load",
		"--statement-id", "2609",
		"--date", "2026-09-22",
		"--bucket", "savings",
		"--category", "investments",
		"--description", "Savings",
		"--amount", "-1000.00",
	})
	if err != nil {
		t.Fatalf("savings transaction load error = %v", err)
	}

	out.Reset()
	err = app.Run([]string{
		"--db", dbPath,
		"statement", "search",
		"--account", "cheque",
		"--top-10",
	})
	if err != nil {
		t.Fatalf("statement search error = %v", err)
	}

	rendered := out.String()
	for _, want := range []string{"Statement search", "income", "balance", "costs", "earnings", "savings", "cost", "income", "savings", "Groceries", "Market", "-42.10", "42.10", "3,957.90", "4,957.90", "1,000.00"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("search output missing %q:\n%s", want, rendered)
		}
	}
}

func TestTransactionLoadCreatesStatementForTransactionDate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "budgeter.db")

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := New(&out, &errOut)

	err := app.Run([]string{
		"--db", dbPath,
		"statement", "tx", "load",
		"--date", "2026-08-23",
		"--bucket", "cost",
		"--category", "groceries",
		"--description", "Market",
		"--amount", "-42.10",
	})
	if err != nil {
		t.Fatalf("transaction load error = %v", err)
	}
	if !strings.Contains(out.String(), "Loaded transaction for statement #2608 (created statement)") {
		t.Fatalf("transaction load output missing created statement:\n%s", out.String())
	}

	out.Reset()
	err = app.Run([]string{
		"--db", dbPath,
		"statement", "show",
		"2608",
		"--all",
	})
	if err != nil {
		t.Fatalf("auto-created statement show error = %v", err)
	}
	for _, want := range []string{"statement #2608", "Market", "Groceries", "-42.10"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("auto-created statement output missing %q:\n%s", want, out.String())
		}
	}
}

func TestTransactionCSVLoadCreatesStatementsByTransactionDate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "budgeter.db")
	csvPath := filepath.Join(t.TempDir(), "txs.csv")
	if err := os.WriteFile(csvPath, []byte(strings.Join([]string{
		"date,bucket,category,desc,amount",
		"2026-08-23,cost,Groceries,Market,-42.10",
		"2026-09-21,income,Miscellaneous,Payroll,5000.00",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := New(&out, &errOut)

	err := app.Run([]string{
		"--db", dbPath,
		"statement", "tx", "load",
		"--file", csvPath,
	})
	if err != nil {
		t.Fatalf("csv transaction load error = %v", err)
	}
	if !strings.Contains(out.String(), "Loaded 2 transaction(s) across 2 statement(s); created 2 statement(s)") {
		t.Fatalf("csv transaction load output missing created statements:\n%s", out.String())
	}

	for _, statementID := range []string{"2608", "2609"} {
		out.Reset()
		err = app.Run([]string{
			"--db", dbPath,
			"statement", "show",
			statementID,
			"--all",
		})
		if err != nil {
			t.Fatalf("auto-created statement %s show error = %v", statementID, err)
		}
		if !strings.Contains(out.String(), "statement #"+statementID) {
			t.Fatalf("auto-created statement %s output missing statement id:\n%s", statementID, out.String())
		}
	}
}

func TestTransactionLoadRejectsStatementIDThatDoesNotMatchTransactionDate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "budgeter.db")

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := New(&out, &errOut)

	err := app.Run([]string{
		"--db", dbPath,
		"statement", "tx", "load",
		"--statement-id", "2609",
		"--date", "2026-09-20",
		"--bucket", "income",
		"--category", "miscellaneous",
		"--description", "Payroll",
		"--amount", "5000.00",
	})
	if err == nil {
		t.Fatal("transaction load mismatched statement id error = nil, want error")
	}
	if !strings.Contains(err.Error(), "expected statement #2608") {
		t.Fatalf("transaction load mismatched statement id error = %q, want expected statement guidance", err.Error())
	}
}

func TestStatementList(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "budgeter.db")

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := New(&out, &errOut)

	loads := [][]string{
		{
			"--db", dbPath,
			"statement", "load",
			"--date", "2026-08-20",
			"--account", "cheque",
			"--balance", "12000.00",
			"--income", "5000.00",
			"--costs", "2000.00",
			"--earnings", "3000.00",
			"--savings", "1000.00",
		},
		{
			"--db", dbPath,
			"statement", "load",
			"--date", "2026-09-21",
			"--account", "cheque",
			"--balance", "12500.00",
			"--income", "5200.00",
			"--costs", "2100.00",
			"--earnings", "3100.00",
			"--savings", "1200.00",
		},
		{
			"--db", dbPath,
			"statement", "load",
			"--date", "2026-10-22",
			"--account", "credit",
			"--balance", "-500.00",
		},
	}

	for _, args := range loads {
		out.Reset()
		if err := app.Run(args); err != nil {
			t.Fatalf("statement load error = %v", err)
		}
	}

	out.Reset()
	err := app.Run([]string{
		"--db", dbPath,
		"statement", "list",
		"--account", "cheque",
		"--limit", "2",
	})
	if err != nil {
		t.Fatalf("statement list error = %v", err)
	}

	rendered := out.String()
	for _, want := range []string{"Statement list", "2 statement(s)", "cheque", "1,900.00", "2,000.00", "5,200.00"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("list output missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "credit") {
		t.Fatalf("list output included filtered account:\n%s", rendered)
	}
}

func TestStatementDeleteRequiresConfirmationAndDeletesStatement(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "budgeter.db")

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := New(&out, &errOut)

	err := app.Run([]string{
		"--db", dbPath,
		"statement", "load",
		"--date", "2026-08-20",
		"--account", "cheque",
		"--balance", "12000.00",
	})
	if err != nil {
		t.Fatalf("statement load error = %v", err)
	}

	out.Reset()
	err = app.Run([]string{
		"--db", dbPath,
		"statement", "tx", "load",
		"--statement-id", "2607",
		"--date", "2026-08-20",
		"--bucket", "cost",
		"--category", "groceries",
		"--description", "Market",
		"--amount", "-42.10",
	})
	if err != nil {
		t.Fatalf("transaction load error = %v", err)
	}

	out.Reset()
	err = app.Run([]string{
		"--db", dbPath,
		"statement", "delete",
		"2607",
	})
	if err == nil {
		t.Fatal("statement delete without --yes error = nil, want error")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("statement delete error = %q, want --yes guidance", err.Error())
	}

	out.Reset()
	err = app.Run([]string{
		"--db", dbPath,
		"statement", "show",
		"2607",
		"--all",
	})
	if err != nil {
		t.Fatalf("statement should still exist after unconfirmed delete, got error = %v", err)
	}
	if !strings.Contains(out.String(), "Market") {
		t.Fatalf("statement show output missing transaction after unconfirmed delete:\n%s", out.String())
	}

	out.Reset()
	err = app.Run([]string{
		"--db", dbPath,
		"statement", "delete",
		"2607",
		"--yes",
	})
	if err != nil {
		t.Fatalf("statement delete error = %v", err)
	}
	if !strings.Contains(out.String(), "Deleted statement #2607") {
		t.Fatalf("delete output missing success:\n%s", out.String())
	}

	out.Reset()
	err = app.Run([]string{
		"--db", dbPath,
		"statement", "show",
		"2607",
	})
	if err == nil {
		t.Fatal("statement show after delete error = nil, want error")
	}
	if !strings.Contains(err.Error(), "was not found") {
		t.Fatalf("statement show after delete error = %q, want not found", err.Error())
	}
}

func TestStatementLoadRejectsExistingStatementPeriod(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "budgeter.db")

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := New(&out, &errOut)

	err := app.Run([]string{
		"--db", dbPath,
		"statement", "load",
		"--date", "2026-08-21",
		"--account", "cheque",
		"--balance", "12000.00",
	})
	if err != nil {
		t.Fatalf("first statement load error = %v", err)
	}
	if !strings.Contains(out.String(), "Loaded statement #2608 for 2026-08-21 (statement period 2026-08-21 to 2026-09-20)") {
		t.Fatalf("statement load output missing period:\n%s", out.String())
	}

	out.Reset()
	err = app.Run([]string{
		"--db", dbPath,
		"statement", "load",
		"--date", "2026-09-20",
		"--account", "cheque",
		"--balance", "12500.00",
	})
	if err == nil {
		t.Fatal("second statement load for same statement period error = nil, want error")
	}
	if !strings.Contains(err.Error(), "already exists for statement period 2026-08-21 to 2026-09-20") {
		t.Fatalf("duplicate statement period error = %q, want period guidance", err.Error())
	}

	out.Reset()
	err = app.Run([]string{
		"--db", dbPath,
		"statement", "list",
		"--account", "cheque",
		"--limit", "5",
	})
	if err != nil {
		t.Fatalf("statement list error = %v", err)
	}
	if !strings.Contains(out.String(), "1 statement(s)") {
		t.Fatalf("list output after rejected duplicate should have one statement:\n%s", out.String())
	}
}

func TestTransactionLoadRejectsInvalidCategory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "budgeter.db")

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := New(&out, &errOut)

	err := app.Run([]string{
		"--db", dbPath,
		"statement", "load",
		"--date", "2026-08-20",
		"--account", "cheque",
		"--balance", "12000.00",
	})
	if err != nil {
		t.Fatalf("statement load error = %v", err)
	}

	out.Reset()
	err = app.Run([]string{
		"--db", dbPath,
		"statement", "tx", "load",
		"--statement-id", "2607",
		"--date", "2026-08-20",
		"--bucket", "cost",
		"--category", "coffee",
		"--description", "Cafe",
		"--amount", "-42.10",
	})
	if err == nil {
		t.Fatal("transaction load invalid category error = nil, want error")
	}
	if !strings.Contains(err.Error(), "invalid category") {
		t.Fatalf("transaction load invalid category error = %q, want invalid category", err.Error())
	}
}
