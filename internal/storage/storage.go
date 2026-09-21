package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Statement struct {
	ID            int64
	Date          string
	Account       string
	BalanceCents  int64
	IncomeCents   int64
	CostsCents    int64
	EarningsCents int64
	SavingsCents  int64
	Notes         string
}

type Transaction struct {
	ID          int64
	StatementID int64
	Date        string
	Bucket      string
	Category    string
	Description string
	AmountCents int64
}

type TransactionFilter struct {
	Bucket   string
	Category string
}

type StatementFilter struct {
	ID              int64
	AccountContains string
	FromDate        string
	ToDate          string
	MinBalanceCents *int64
	MaxBalanceCents *int64
	Limit           int
}

type TransactionMode string

const (
	TransactionTop10        TransactionMode = "top-10"
	TransactionTop100       TransactionMode = "top-100"
	TransactionAll          TransactionMode = "all"
	TransactionCurrentMonth TransactionMode = "current-month"
)

const (
	BucketIncome  = "income"
	BucketCost    = "cost"
	BucketSavings = "savings"
)

var Categories = []string{
	"Bills",
	"Utilities",
	"Groceries",
	"Eating Out",
	"Subscriptions",
	"Transport",
	"Shopping",
	"Entertainment",
	"Debt Repayments",
	"Investments",
	"Insurance",
	"Healthcare",
	"Personal Care",
	"Miscellaneous",
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".xt", "xt.db"), nil
}

func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return nil, err
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS statements (
			id INTEGER PRIMARY KEY,
			statement_date TEXT NOT NULL,
			account TEXT NOT NULL DEFAULT '',
			balance_cents INTEGER NOT NULL DEFAULT 0,
			income_cents INTEGER NOT NULL DEFAULT 0,
			costs_cents INTEGER NOT NULL DEFAULT 0,
			earnings_cents INTEGER NOT NULL DEFAULT 0,
			savings_cents INTEGER NOT NULL DEFAULT 0,
			notes TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS transactions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			statement_id INTEGER NOT NULL REFERENCES statements(id) ON DELETE CASCADE,
			tx_date TEXT NOT NULL,
			bucket TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			amount_cents INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_statements_date ON statements(statement_date DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_transactions_statement_date ON transactions(statement_id, tx_date DESC, id DESC)`,
	}

	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := s.ensureColumn(ctx, "transactions", "bucket", "bucket TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureColumn(ctx context.Context, table string, column string, definition string) error {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if strings.EqualFold(name, column) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", table, definition))
	return err
}

func (s *Store) LatestStatement(ctx context.Context) (Statement, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, statement_date, account, balance_cents, income_cents, costs_cents, earnings_cents, savings_cents, notes
		FROM statements
		ORDER BY statement_date DESC, id DESC
		LIMIT 1`)

	statement, err := scanStatement(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Statement{}, false, nil
	}
	if err != nil {
		return Statement{}, false, err
	}
	return statement, true, nil
}

func (s *Store) StatementByID(ctx context.Context, id int64) (Statement, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, statement_date, account, balance_cents, income_cents, costs_cents, earnings_cents, savings_cents, notes
		FROM statements
		WHERE id = ?`, id)

	statement, err := scanStatement(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Statement{}, false, nil
	}
	if err != nil {
		return Statement{}, false, err
	}
	return statement, true, nil
}

func (s *Store) StatementForStatementPeriod(ctx context.Context, date string) (Statement, bool, error) {
	start, end, _, err := StatementPeriodRange(date)
	if err != nil {
		return Statement{}, false, err
	}

	row := s.db.QueryRowContext(ctx, `SELECT id, statement_date, account, balance_cents, income_cents, costs_cents, earnings_cents, savings_cents, notes
		FROM statements
		WHERE statement_date >= ? AND statement_date < ?
		ORDER BY statement_date DESC, id DESC
		LIMIT 1`, start, end)

	statement, err := scanStatement(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Statement{}, false, nil
	}
	if err != nil {
		return Statement{}, false, err
	}
	return statement, true, nil
}

func (s *Store) InsertStatement(ctx context.Context, statement Statement) (Statement, error) {
	date, err := NormalizeDate(statement.Date)
	if err != nil {
		return Statement{}, err
	}
	if date == "" {
		date = Today()
	}
	statement.Date = date
	statementID, err := StatementPeriodID(statement.Date)
	if err != nil {
		return Statement{}, err
	}
	statement.ID = statementID

	_, err = s.db.ExecContext(ctx, `INSERT INTO statements (
			id, statement_date, account, balance_cents, income_cents, costs_cents, earnings_cents, savings_cents, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		statement.ID,
		statement.Date,
		statement.Account,
		statement.BalanceCents,
		statement.IncomeCents,
		statement.CostsCents,
		statement.EarningsCents,
		statement.SavingsCents,
		statement.Notes,
	)
	if err != nil {
		return Statement{}, err
	}
	return statement, nil
}

func (s *Store) InsertTransactions(ctx context.Context, transactions []Transaction) error {
	if len(transactions) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO transactions (
			statement_id, tx_date, bucket, category, description, amount_cents
		) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, transaction := range transactions {
		date, err := NormalizeDate(transaction.Date)
		if err != nil {
			return err
		}
		if date == "" {
			date = Today()
		}
		bucket, err := NormalizeBucket(transaction.Bucket)
		if err != nil {
			return err
		}
		if bucket == "" {
			return fmt.Errorf("transaction bucket is required; use %s, %s, or %s", BucketIncome, BucketCost, BucketSavings)
		}
		category, err := NormalizeCategory(transaction.Category)
		if err != nil {
			return err
		}
		if category == "" {
			return fmt.Errorf("transaction category is required; use one of: %s", CategoriesText())
		}

		if _, err := stmt.ExecContext(
			ctx,
			transaction.StatementID,
			date,
			bucket,
			category,
			transaction.Description,
			transaction.AmountCents,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) StatementWithCalculatedRows(ctx context.Context, statement Statement) (Statement, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT bucket, amount_cents FROM transactions WHERE statement_id = ? AND bucket <> ''`, statement.ID)
	if err != nil {
		return Statement{}, err
	}
	defer rows.Close()

	var income, costs, savings int64
	hasBucketedTransactions := false
	for rows.Next() {
		var bucket string
		var amount int64
		if err := rows.Scan(&bucket, &amount); err != nil {
			return Statement{}, err
		}

		normalized, err := NormalizeBucket(bucket)
		if err != nil {
			return Statement{}, err
		}
		switch normalized {
		case BucketIncome:
			income += amount
			hasBucketedTransactions = true
		case BucketCost:
			costs += absolute(amount)
			hasBucketedTransactions = true
		case BucketSavings:
			savings += absolute(amount)
			hasBucketedTransactions = true
		}
	}
	if err := rows.Err(); err != nil {
		return Statement{}, err
	}
	if !hasBucketedTransactions {
		income = statement.IncomeCents
		costs = statement.CostsCents
		savings = statement.SavingsCents
	}

	statement.IncomeCents = income
	statement.CostsCents = costs
	statement.EarningsCents = income - costs
	statement.SavingsCents = savings
	statement.BalanceCents = income - costs - savings
	return statement, nil
}

func (s *Store) DeleteStatement(ctx context.Context, id int64) (bool, error) {
	if id <= 0 {
		return false, fmt.Errorf("statement id is required")
	}

	result, err := s.db.ExecContext(ctx, `DELETE FROM statements WHERE id = ?`, id)
	if err != nil {
		return false, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *Store) SearchStatements(ctx context.Context, filter StatementFilter) ([]Statement, error) {
	var clauses []string
	var args []any

	if filter.ID > 0 {
		clauses = append(clauses, "id = ?")
		args = append(args, filter.ID)
	}
	if filter.AccountContains != "" {
		clauses = append(clauses, "LOWER(account) LIKE ?")
		args = append(args, "%"+strings.ToLower(filter.AccountContains)+"%")
	}
	if filter.FromDate != "" {
		clauses = append(clauses, "statement_date >= ?")
		args = append(args, filter.FromDate)
	}
	if filter.ToDate != "" {
		clauses = append(clauses, "statement_date <= ?")
		args = append(args, filter.ToDate)
	}
	if filter.MinBalanceCents != nil {
		clauses = append(clauses, "balance_cents >= ?")
		args = append(args, *filter.MinBalanceCents)
	}
	if filter.MaxBalanceCents != nil {
		clauses = append(clauses, "balance_cents <= ?")
		args = append(args, *filter.MaxBalanceCents)
	}

	query := `SELECT id, statement_date, account, balance_cents, income_cents, costs_cents, earnings_cents, savings_cents, notes FROM statements`
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY statement_date DESC, id DESC"
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var statements []Statement
	for rows.Next() {
		var statement Statement
		if err := rows.Scan(
			&statement.ID,
			&statement.Date,
			&statement.Account,
			&statement.BalanceCents,
			&statement.IncomeCents,
			&statement.CostsCents,
			&statement.EarningsCents,
			&statement.SavingsCents,
			&statement.Notes,
		); err != nil {
			return nil, err
		}
		statements = append(statements, statement)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return statements, nil
}

func (s *Store) Transactions(ctx context.Context, statementID int64, mode TransactionMode) ([]Transaction, error) {
	return s.TransactionsFiltered(ctx, statementID, mode, TransactionFilter{})
}

func (s *Store) TransactionsFiltered(ctx context.Context, statementID int64, mode TransactionMode, filter TransactionFilter) ([]Transaction, error) {
	if statementID <= 0 {
		return nil, fmt.Errorf("statement id is required")
	}

	query := `SELECT id, statement_id, tx_date, bucket, category, description, amount_cents
		FROM transactions
		WHERE statement_id = ?`
	args := []any{statementID}

	if filter.Bucket != "" {
		bucket, err := NormalizeBucket(filter.Bucket)
		if err != nil {
			return nil, err
		}
		query += " AND bucket = ?"
		args = append(args, bucket)
	}
	if filter.Category != "" {
		category, err := NormalizeCategory(filter.Category)
		if err != nil {
			return nil, err
		}
		query += " AND category = ?"
		args = append(args, category)
	}

	if mode == TransactionCurrentMonth {
		start, end := CurrentMonthRange()
		query += " AND tx_date >= ? AND tx_date < ?"
		args = append(args, start, end)
	}

	query += " ORDER BY tx_date DESC, id DESC"
	switch mode {
	case TransactionTop10, "":
		query += " LIMIT 10"
	case TransactionTop100:
		query += " LIMIT 100"
	case TransactionAll, TransactionCurrentMonth:
	default:
		return nil, fmt.Errorf("unknown transaction mode %q", mode)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []Transaction
	for rows.Next() {
		var transaction Transaction
		if err := rows.Scan(
			&transaction.ID,
			&transaction.StatementID,
			&transaction.Date,
			&transaction.Bucket,
			&transaction.Category,
			&transaction.Description,
			&transaction.AmountCents,
		); err != nil {
			return nil, err
		}
		transactions = append(transactions, transaction)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return transactions, nil
}

func (s *Store) StatementExists(ctx context.Context, id int64) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM statements WHERE id = ?`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func NormalizeBucket(input string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "":
		return "", nil
	case BucketIncome:
		return BucketIncome, nil
	case BucketCost, "costs":
		return BucketCost, nil
	case BucketSavings, "saving":
		return BucketSavings, nil
	default:
		return "", fmt.Errorf("invalid bucket %q; use %s, %s, or %s", input, BucketIncome, BucketCost, BucketSavings)
	}
}

func NormalizeCategory(input string) (string, error) {
	normalized := normalizeCategoryKey(input)
	if normalized == "" {
		return "", nil
	}

	for _, category := range Categories {
		if normalizeCategoryKey(category) == normalized {
			return category, nil
		}
	}

	return "", fmt.Errorf("invalid category %q; use one of: %s", input, CategoriesText())
}

func CategoriesText() string {
	return strings.Join(Categories, ", ")
}

func normalizeCategoryKey(input string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(input))), " ")
}

func absolute(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

type scanner interface {
	Scan(dest ...any) error
}

func scanStatement(row scanner) (Statement, error) {
	var statement Statement
	err := row.Scan(
		&statement.ID,
		&statement.Date,
		&statement.Account,
		&statement.BalanceCents,
		&statement.IncomeCents,
		&statement.CostsCents,
		&statement.EarningsCents,
		&statement.SavingsCents,
		&statement.Notes,
	)
	return statement, err
}
