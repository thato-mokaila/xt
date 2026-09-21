package ui

import (
	"fmt"
	"strings"

	"xt/internal/storage"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type StatementModel struct {
	title        string
	statements   []storage.Statement
	statement    storage.Statement
	transactions []storage.Transaction
	txMode       storage.TransactionMode
	statementTbl table.Model
	txTbl        table.Model
}

type StatementListModel struct {
	title      string
	statements []storage.Statement
	tbl        table.Model
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("57")).
			Padding(0, 1)
	subtleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	labelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	valueStyle  = lipgloss.NewStyle().Bold(true)
	boxStyle    = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(1, 2)
)

func NewStatementModel(title string, statements []storage.Statement, statement storage.Statement, transactions []storage.Transaction, mode storage.TransactionMode) StatementModel {
	statementTbl := table.New(
		table.WithColumns([]table.Column{
			{Title: "ID", Width: 6},
			{Title: "Date", Width: 12},
			{Title: "Account", Width: 22},
			{Title: "Balance", Width: 14},
		}),
		table.WithRows(statementRows(statements)),
		table.WithHeight(min(max(len(statements)+1, 3), 6)),
	)
	statementTbl.SetStyles(tableStyles())

	txTbl := table.New(
		table.WithColumns([]table.Column{
			{Title: "Date", Width: 12},
			{Title: "Bucket", Width: 10},
			{Title: "Category", Width: 16},
			{Title: "Desc", Width: 30},
			{Title: "Amount", Width: 14},
		}),
		table.WithRows(transactionRows(transactions)),
		table.WithFocused(true),
		table.WithHeight(min(max(len(transactions)+1, 3), 12)),
	)
	txTbl.SetStyles(tableStyles())

	return StatementModel{
		title:        title,
		statements:   statements,
		statement:    statement,
		transactions: transactions,
		txMode:       mode,
		statementTbl: statementTbl,
		txTbl:        txTbl,
	}
}

func (m StatementModel) Init() tea.Cmd {
	return nil
}

func (m StatementModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.txTbl, cmd = m.txTbl.Update(msg)
	return m, cmd
}

func (m StatementModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.title))
	b.WriteString("\n")
	b.WriteString(subtleStyle.Render(fmt.Sprintf("statement #%d  %s  %s", m.statement.ID, m.statement.Date, empty(m.statement.Account, "no account"))))
	if m.txMode != "" {
		b.WriteString(subtleStyle.Render("  tx: " + string(m.txMode)))
	}
	b.WriteString("\n\n")

	if len(m.statements) > 1 {
		b.WriteString(labelStyle.Render("Matched statements"))
		b.WriteString("\n")
		b.WriteString(m.statementTbl.View())
		b.WriteString("\n\n")
	}

	b.WriteString(labelStyle.Render("Statement rows"))
	b.WriteString("\n")
	b.WriteString(boxStyle.Render(summaryRows(m.statement)))
	b.WriteString("\n\n")

	b.WriteString(labelStyle.Render("Transactions"))
	b.WriteString("\n")
	if len(m.transactions) == 0 {
		b.WriteString(subtleStyle.Render("No transactions found."))
	} else {
		b.WriteString(m.txTbl.View())
	}
	b.WriteString("\n\n")
	b.WriteString(subtleStyle.Render("Use arrow keys to scroll transactions. Press q to quit."))
	return b.String()
}

func StaticStatementView(title string, statements []storage.Statement, statement storage.Statement, transactions []storage.Transaction, mode storage.TransactionMode) string {
	model := NewStatementModel(title, statements, statement, transactions, mode)
	return model.View()
}

func NewStatementListModel(title string, statements []storage.Statement) StatementListModel {
	tbl := table.New(
		table.WithColumns([]table.Column{
			{Title: "ID", Width: 6},
			{Title: "Date", Width: 12},
			{Title: "Account", Width: 20},
			{Title: "Income", Width: 13},
			{Title: "Balance", Width: 14},
			{Title: "Costs", Width: 13},
			{Title: "Earnings", Width: 13},
			{Title: "Savings", Width: 13},
		}),
		table.WithRows(statementListRows(statements)),
		table.WithFocused(true),
		table.WithHeight(min(max(len(statements)+1, 3), 18)),
	)
	tbl.SetStyles(tableStyles())

	return StatementListModel{
		title:      title,
		statements: statements,
		tbl:        tbl,
	}
}

func (m StatementListModel) Init() tea.Cmd {
	return nil
}

func (m StatementListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.tbl, cmd = m.tbl.Update(msg)
	return m, cmd
}

func (m StatementListModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.title))
	b.WriteString("\n")
	b.WriteString(subtleStyle.Render(fmt.Sprintf("%d statement(s)", len(m.statements))))
	b.WriteString("\n\n")
	b.WriteString(m.tbl.View())
	b.WriteString("\n\n")
	b.WriteString(subtleStyle.Render("Use arrow keys to scroll statements. Press q to quit."))
	return b.String()
}

func StaticStatementListView(title string, statements []storage.Statement) string {
	model := NewStatementListModel(title, statements)
	return model.View()
}

func tableStyles() table.Styles {
	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("238")).
		BorderBottom(true).
		Bold(true).
		Foreground(lipgloss.Color("230"))
	styles.Selected = styles.Selected.
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("57")).
		Bold(false)
	return styles
}

func statementRows(statements []storage.Statement) []table.Row {
	rows := make([]table.Row, 0, len(statements))
	for _, statement := range statements {
		rows = append(rows, table.Row{
			fmt.Sprintf("%d", statement.ID),
			statement.Date,
			truncate(empty(statement.Account, "-"), 22),
			storage.FormatMoney(statement.BalanceCents),
		})
	}
	if len(rows) == 0 {
		rows = append(rows, table.Row{"-", "-", "-", "-"})
	}
	return rows
}

func statementListRows(statements []storage.Statement) []table.Row {
	rows := make([]table.Row, 0, len(statements))
	for _, statement := range statements {
		rows = append(rows, table.Row{
			fmt.Sprintf("%d", statement.ID),
			statement.Date,
			truncate(empty(statement.Account, "-"), 20),
			storage.FormatMoney(statement.IncomeCents),
			storage.FormatMoney(statement.BalanceCents),
			storage.FormatMoney(statement.CostsCents),
			storage.FormatMoney(statement.EarningsCents),
			storage.FormatMoney(statement.SavingsCents),
		})
	}
	if len(rows) == 0 {
		rows = append(rows, table.Row{"-", "-", "-", "-", "-", "-", "-", "-"})
	}
	return rows
}

func transactionRows(transactions []storage.Transaction) []table.Row {
	rows := make([]table.Row, 0, len(transactions))
	for _, transaction := range transactions {
		rows = append(rows, table.Row{
			transaction.Date,
			truncate(empty(transaction.Bucket, "-"), 10),
			truncate(empty(transaction.Category, "-"), 16),
			truncate(empty(transaction.Description, "-"), 30),
			storage.FormatMoney(transaction.AmountCents),
		})
	}
	if len(rows) == 0 {
		rows = append(rows, table.Row{"-", "-", "-", "-", "-"})
	}
	return rows
}

func summaryRows(statement storage.Statement) string {
	rows := []struct {
		label string
		value int64
	}{
		{"income", statement.IncomeCents},
		{"balance", statement.BalanceCents},
		{"costs", statement.CostsCents},
		{"earnings", statement.EarningsCents},
		{"savings", statement.SavingsCents},
	}

	width := 0
	for _, row := range rows {
		if len(row.label) > width {
			width = len(row.label)
		}
	}

	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, fmt.Sprintf("%-*s  %s", width, row.label, valueStyle.Render(storage.FormatMoney(row.value))))
	}
	return strings.Join(lines, "\n")
}

func empty(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func truncate(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
