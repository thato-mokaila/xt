package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreStatementSearchAndTransactions(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "budgeter.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	statement, err := store.InsertStatement(ctx, Statement{
		Date:          "2026-08-20",
		Account:       "cheque",
		BalanceCents:  1200000,
		IncomeCents:   500000,
		CostsCents:    200000,
		EarningsCents: 300000,
		SavingsCents:  100000,
	})
	if err != nil {
		t.Fatalf("InsertStatement() error = %v", err)
	}
	if statement.ID != 2607 {
		t.Fatalf("statement.ID = %d, want 2607", statement.ID)
	}

	err = store.InsertTransactions(ctx, []Transaction{
		{StatementID: statement.ID, Date: "2026-08-20", Bucket: BucketCost, Category: "groceries", Description: "Market", AmountCents: -4210},
		{StatementID: statement.ID, Date: "2026-08-19", Bucket: BucketIncome, Category: "miscellaneous", Description: "Payroll", AmountCents: 500000},
		{StatementID: statement.ID, Date: "2026-08-18", Bucket: BucketSavings, Category: "investments", Description: "Savings", AmountCents: -100000},
	})
	if err != nil {
		t.Fatalf("InsertTransactions() error = %v", err)
	}

	matches, err := store.SearchStatements(ctx, StatementFilter{AccountContains: "che", Limit: 10})
	if err != nil {
		t.Fatalf("SearchStatements() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("SearchStatements() returned %d statements, want 1", len(matches))
	}
	if matches[0].BalanceCents != 1200000 {
		t.Fatalf("BalanceCents = %d, want 1200000", matches[0].BalanceCents)
	}

	transactions, err := store.Transactions(ctx, statement.ID, TransactionTop10)
	if err != nil {
		t.Fatalf("Transactions() error = %v", err)
	}
	if len(transactions) != 3 {
		t.Fatalf("Transactions() returned %d rows, want 3", len(transactions))
	}
	if transactions[0].Date != "2026-08-20" || transactions[0].Description != "Market" {
		t.Fatalf("Transactions()[0] = %#v, want latest transaction", transactions[0])
	}
	if transactions[0].Bucket != BucketCost {
		t.Fatalf("Transactions()[0].Bucket = %q, want %q", transactions[0].Bucket, BucketCost)
	}
	if transactions[0].Category != "Groceries" {
		t.Fatalf("Transactions()[0].Category = %q, want Groceries", transactions[0].Category)
	}

	filteredTransactions, err := store.TransactionsFiltered(ctx, statement.ID, TransactionAll, TransactionFilter{
		Bucket:   "cost",
		Category: "groceries",
	})
	if err != nil {
		t.Fatalf("TransactionsFiltered() error = %v", err)
	}
	if len(filteredTransactions) != 1 {
		t.Fatalf("TransactionsFiltered() returned %d rows, want 1", len(filteredTransactions))
	}
	if filteredTransactions[0].Description != "Market" {
		t.Fatalf("TransactionsFiltered()[0].Description = %q, want Market", filteredTransactions[0].Description)
	}

	calculated, err := store.StatementWithCalculatedRows(ctx, statement)
	if err != nil {
		t.Fatalf("StatementWithCalculatedRows() error = %v", err)
	}
	if calculated.IncomeCents != 500000 {
		t.Fatalf("calculated income = %d, want 500000", calculated.IncomeCents)
	}
	if calculated.CostsCents != 4210 {
		t.Fatalf("calculated costs = %d, want 4210", calculated.CostsCents)
	}
	if calculated.EarningsCents != 495790 {
		t.Fatalf("calculated earnings = %d, want 495790", calculated.EarningsCents)
	}
	if calculated.SavingsCents != 100000 {
		t.Fatalf("calculated savings = %d, want 100000", calculated.SavingsCents)
	}
	if calculated.BalanceCents != 395790 {
		t.Fatalf("calculated balance = %d, want 395790", calculated.BalanceCents)
	}

	monthlyStatement, ok, err := store.StatementForStatementPeriod(ctx, "2026-08-01")
	if err != nil {
		t.Fatalf("StatementForStatementPeriod() error = %v", err)
	}
	if !ok {
		t.Fatal("StatementForStatementPeriod() ok = false, want true")
	}
	if monthlyStatement.ID != statement.ID {
		t.Fatalf("StatementForStatementPeriod() id = %d, want %d", monthlyStatement.ID, statement.ID)
	}
}

func TestDeleteStatementCascadesTransactions(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "budgeter.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	statement, err := store.InsertStatement(ctx, Statement{
		Date:         "2026-08-20",
		Account:      "cheque",
		BalanceCents: 1200000,
	})
	if err != nil {
		t.Fatalf("InsertStatement() error = %v", err)
	}

	err = store.InsertTransactions(ctx, []Transaction{
		{StatementID: statement.ID, Date: "2026-08-20", Bucket: BucketCost, Category: "Groceries", Description: "Market", AmountCents: -4210},
	})
	if err != nil {
		t.Fatalf("InsertTransactions() error = %v", err)
	}

	deleted, err := store.DeleteStatement(ctx, statement.ID)
	if err != nil {
		t.Fatalf("DeleteStatement() error = %v", err)
	}
	if !deleted {
		t.Fatal("DeleteStatement() deleted = false, want true")
	}

	if _, ok, err := store.StatementByID(ctx, statement.ID); err != nil {
		t.Fatalf("StatementByID() error = %v", err)
	} else if ok {
		t.Fatal("StatementByID() found deleted statement")
	}

	transactions, err := store.Transactions(ctx, statement.ID, TransactionAll)
	if err != nil {
		t.Fatalf("Transactions() error = %v", err)
	}
	if len(transactions) != 0 {
		t.Fatalf("Transactions() returned %d rows, want 0", len(transactions))
	}
}

func TestStatementPeriodID(t *testing.T) {
	tests := map[string]int64{
		"2026-08-20": 2607,
		"2026-08-21": 2608,
		"2026-09-20": 2608,
		"2026-09-21": 2609,
	}

	for input, want := range tests {
		got, err := StatementPeriodID(input)
		if err != nil {
			t.Fatalf("StatementPeriodID(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("StatementPeriodID(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestNormalizeCategory(t *testing.T) {
	tests := map[string]string{
		"groceries":        "Groceries",
		"EATING   OUT":     "Eating Out",
		"debt repayments":  "Debt Repayments",
		" personal care  ": "Personal Care",
	}

	for input, want := range tests {
		got, err := NormalizeCategory(input)
		if err != nil {
			t.Fatalf("NormalizeCategory(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizeCategory(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestInsertTransactionsRejectsInvalidCategory(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "budgeter.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	statement, err := store.InsertStatement(ctx, Statement{
		Date:         "2026-08-20",
		Account:      "cheque",
		BalanceCents: 1200000,
	})
	if err != nil {
		t.Fatalf("InsertStatement() error = %v", err)
	}

	err = store.InsertTransactions(ctx, []Transaction{
		{StatementID: statement.ID, Date: "2026-08-20", Bucket: BucketCost, Category: "coffee", Description: "Cafe", AmountCents: -4210},
	})
	if err == nil {
		t.Fatal("InsertTransactions() error = nil, want invalid category error")
	}
	if !strings.Contains(err.Error(), "invalid category") {
		t.Fatalf("InsertTransactions() error = %q, want invalid category", err.Error())
	}
}
