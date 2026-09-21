package storage

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
)

func LoadStatementsCSV(path string, latest Statement, hasLatest bool) ([]Statement, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read statement csv header: %w", err)
	}
	headerIndex := indexHeader(header)

	base := Statement{Date: Today()}
	if hasLatest {
		base = latest
		base.ID = 0
		base.Date = Today()
	}

	var statements []Statement
	line := 1
	for {
		line++
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read statement csv line %d: %w", line, err)
		}
		if blankRecord(record) {
			continue
		}

		statement := base
		statement.ID = 0
		if err := applyStatementRecord(&statement, headerIndex, record); err != nil {
			return nil, fmt.Errorf("statement csv line %d: %w", line, err)
		}
		statements = append(statements, statement)
		base = statement
	}

	return statements, nil
}

func LoadTransactionsCSV(path string, statementID int64) ([]Transaction, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read transaction csv header: %w", err)
	}
	headerIndex := indexHeader(header)

	var transactions []Transaction
	line := 1
	for {
		line++
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read transaction csv line %d: %w", line, err)
		}
		if blankRecord(record) {
			continue
		}

		transaction := Transaction{StatementID: statementID, Date: Today()}
		if err := applyTransactionRecord(&transaction, headerIndex, record); err != nil {
			return nil, fmt.Errorf("transaction csv line %d: %w", line, err)
		}
		transactions = append(transactions, transaction)
	}

	return transactions, nil
}

func applyStatementRecord(statement *Statement, header map[string]int, record []string) error {
	for _, name := range []string{"date", "statement_date"} {
		if value, ok := csvValue(header, record, name); ok && value != "" {
			date, err := NormalizeDate(value)
			if err != nil {
				return err
			}
			statement.Date = date
			break
		}
	}
	if value, ok := csvValue(header, record, "account"); ok && value != "" {
		statement.Account = value
	}
	if value, ok := csvValue(header, record, "notes"); ok {
		statement.Notes = value
	}

	moneyFields := []struct {
		name string
		set  func(int64)
	}{
		{"balance", func(v int64) { statement.BalanceCents = v }},
		{"income", func(v int64) { statement.IncomeCents = v }},
		{"costs", func(v int64) { statement.CostsCents = v }},
		{"earnings", func(v int64) { statement.EarningsCents = v }},
		{"savings", func(v int64) { statement.SavingsCents = v }},
	}

	for _, field := range moneyFields {
		value, ok := csvValue(header, record, field.name)
		if !ok || value == "" {
			continue
		}
		cents, err := ParseMoney(value)
		if err != nil {
			return err
		}
		field.set(cents)
	}

	return nil
}

func applyTransactionRecord(transaction *Transaction, header map[string]int, record []string) error {
	for _, name := range []string{"date", "tx_date", "transaction_date"} {
		if value, ok := csvValue(header, record, name); ok && value != "" {
			date, err := NormalizeDate(value)
			if err != nil {
				return err
			}
			transaction.Date = date
			break
		}
	}
	if value, ok := csvValue(header, record, "category"); ok {
		category, err := NormalizeCategory(value)
		if err != nil {
			return err
		}
		transaction.Category = category
	}
	if value, ok := csvValue(header, record, "bucket"); ok {
		bucket, err := NormalizeBucket(value)
		if err != nil {
			return err
		}
		transaction.Bucket = bucket
	}
	for _, name := range []string{"description", "desc"} {
		if value, ok := csvValue(header, record, name); ok {
			transaction.Description = value
			break
		}
	}
	if value, ok := csvValue(header, record, "amount"); ok && value != "" {
		cents, err := ParseMoney(value)
		if err != nil {
			return err
		}
		transaction.AmountCents = cents
	}

	return nil
}

func indexHeader(header []string) map[string]int {
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[strings.ToLower(strings.TrimSpace(name))] = i
	}
	return index
}

func csvValue(header map[string]int, record []string, name string) (string, bool) {
	index, ok := header[name]
	if !ok || index >= len(record) {
		return "", false
	}
	return strings.TrimSpace(record[index]), true
}

func blankRecord(record []string) bool {
	for _, field := range record {
		if strings.TrimSpace(field) != "" {
			return false
		}
	}
	return true
}
