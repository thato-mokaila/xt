package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"xt/internal/storage"
	"xt/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type CLI struct {
	out io.Writer
	err io.Writer
}

type config struct {
	dbPath string
}

var successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)

func New(out io.Writer, err io.Writer) *CLI {
	return &CLI{out: out, err: err}
}

func (c *CLI) Run(args []string) error {
	cfg, rest, err := parseGlobal(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		c.printUsage()
		return nil
	}
	if rest[0] == "help" || rest[0] == "--help" || rest[0] == "-h" {
		c.printUsage()
		return nil
	}
	if rest[0] == "statement" || rest[0] == "statements" {
		if len(rest) == 1 || rest[1] == "help" || rest[1] == "--help" || rest[1] == "-h" {
			c.printStatementUsage()
			return nil
		}
	}

	ctx := context.Background()
	store, err := storage.Open(ctx, cfg.dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	switch rest[0] {
	case "statement", "statements":
		return c.runStatement(ctx, store, rest[1:])
	default:
		return fmt.Errorf("unknown command %q\n\n%s", rest[0], usage())
	}
}

func parseGlobal(args []string) (config, []string, error) {
	cfg := config{}
	for len(args) > 0 {
		switch {
		case args[0] == "--db":
			if len(args) < 2 {
				return cfg, nil, fmt.Errorf("--db requires a path")
			}
			cfg.dbPath = args[1]
			args = args[2:]
		case strings.HasPrefix(args[0], "--db="):
			cfg.dbPath = strings.TrimPrefix(args[0], "--db=")
			args = args[1:]
		case args[0] == "-h" || args[0] == "--help":
			return cfg, args, nil
		default:
			return cfg, args, nil
		}
	}
	return cfg, args, nil
}

func (c *CLI) runStatement(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) == 0 {
		c.printStatementUsage()
		return nil
	}

	switch args[0] {
	case "load":
		return c.runStatementLoad(ctx, store, args[1:])
	case "list":
		return c.runStatementList(ctx, store, args[1:])
	case "delete":
		return c.runStatementDelete(ctx, store, args[1:])
	case "search":
		return c.runStatementSearch(ctx, store, args[1:])
	case "show":
		return c.runStatementShow(ctx, store, args[1:])
	case "tx", "transactions":
		return c.runTransactions(ctx, store, args[1:])
	case "help", "--help", "-h":
		c.printStatementUsage()
		return nil
	default:
		return fmt.Errorf("unknown statement command %q\n\n%s", args[0], statementUsage())
	}
}

func (c *CLI) runStatementLoad(ctx context.Context, store *storage.Store, args []string) error {
	fs := c.flagSet("statement load")
	file := fs.String("file", "", "CSV file with statement rows")
	date := fs.String("date", "", "statement date YYYY-MM-DD")
	account := fs.String("account", "", "account name")
	balance := fs.String("balance", "", "latest balance")
	income := fs.String("income", "", "income")
	costs := fs.String("costs", "", "costs")
	earnings := fs.String("earnings", "", "earnings")
	savings := fs.String("savings", "", "savings")
	notes := fs.String("notes", "", "notes")
	if err := fs.Parse(args); err != nil {
		return err
	}

	latest, hasLatest, err := store.LatestStatement(ctx)
	if err != nil {
		return err
	}

	if *file != "" {
		statements, err := storage.LoadStatementsCSV(*file, latest, hasLatest)
		if err != nil {
			return err
		}
		if len(statements) == 0 {
			return fmt.Errorf("no statements found in %s", *file)
		}
		if err := ensureStatementLoadPeriodsAvailable(ctx, store, statements); err != nil {
			return err
		}

		for i := range statements {
			inserted, err := store.InsertStatement(ctx, statements[i])
			if err != nil {
				return err
			}
			statements[i] = inserted
		}
		fmt.Fprintf(c.out, "%s Loaded %d statement(s). Latest statement id: %d\n", successStyle.Render("OK"), len(statements), statements[len(statements)-1].ID)
		return nil
	}

	statement := storage.Statement{Date: storage.Today()}
	if hasLatest {
		statement = latest
		statement.ID = 0
		statement.Date = storage.Today()
	}

	visited := visitedFlags(fs)
	if visited["date"] {
		statement.Date, err = storage.NormalizeDate(*date)
		if err != nil {
			return err
		}
	}
	if visited["account"] {
		statement.Account = *account
	}
	if visited["notes"] {
		statement.Notes = *notes
	}
	moneyArgs := []struct {
		name  string
		value string
		set   func(int64)
	}{
		{"balance", *balance, func(v int64) { statement.BalanceCents = v }},
		{"income", *income, func(v int64) { statement.IncomeCents = v }},
		{"costs", *costs, func(v int64) { statement.CostsCents = v }},
		{"earnings", *earnings, func(v int64) { statement.EarningsCents = v }},
		{"savings", *savings, func(v int64) { statement.SavingsCents = v }},
	}
	for _, arg := range moneyArgs {
		if !visited[arg.name] {
			continue
		}
		cents, err := storage.ParseMoney(arg.value)
		if err != nil {
			return err
		}
		arg.set(cents)
	}
	if err := ensureStatementLoadPeriodsAvailable(ctx, store, []storage.Statement{statement}); err != nil {
		return err
	}

	inserted, err := store.InsertStatement(ctx, statement)
	if err != nil {
		return err
	}
	_, _, period, err := storage.StatementPeriodRange(inserted.Date)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "%s Loaded statement #%d for %s (statement period %s).\n", successStyle.Render("OK"), inserted.ID, inserted.Date, period)
	return nil
}

func (c *CLI) runStatementList(ctx context.Context, store *storage.Store, args []string) error {
	fs := c.flagSet("statement list")
	id := fs.Int64("id", 0, "exact statement id")
	account := fs.String("account", "", "account search text")
	from := fs.String("from", "", "from date YYYY-MM-DD")
	to := fs.String("to", "", "to date YYYY-MM-DD")
	minBalance := fs.String("min-balance", "", "minimum balance")
	maxBalance := fs.String("max-balance", "", "maximum balance")
	limit := fs.Int("limit", 25, "maximum statements to list")
	if err := fs.Parse(args); err != nil {
		return err
	}

	filter, err := buildStatementFilter(*id, *account, *from, *to, *minBalance, *maxBalance, *limit)
	if err != nil {
		return err
	}

	statements, err := store.SearchStatements(ctx, filter)
	if err != nil {
		return err
	}
	if len(statements) == 0 {
		fmt.Fprintln(c.out, "No statements found.")
		return nil
	}
	statements, err = c.calculatedStatements(ctx, store, statements)
	if err != nil {
		return err
	}

	return c.renderStatementList("Statement list", statements)
}

func (c *CLI) runStatementDelete(ctx context.Context, store *storage.Store, args []string) error {
	positionalID, remainingArgs, err := leadingStatementID(args)
	if err != nil {
		return err
	}

	fs := c.flagSet("statement delete")
	id := fs.Int64("id", positionalID, "statement id")
	yes := fs.Bool("yes", false, "confirm deletion")
	if err := fs.Parse(remainingArgs); err != nil {
		return err
	}

	if *id == 0 && fs.NArg() > 0 {
		parsed, err := strconv.ParseInt(fs.Arg(0), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid statement id %q", fs.Arg(0))
		}
		*id = parsed
	}
	if *id == 0 {
		return fmt.Errorf("statement id is required")
	}
	if !*yes {
		return fmt.Errorf("deleting statement #%d also deletes its transactions; rerun with --yes to confirm", *id)
	}

	statement, ok, err := store.StatementByID(ctx, *id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("statement #%d was not found", *id)
	}

	deleted, err := store.DeleteStatement(ctx, *id)
	if err != nil {
		return err
	}
	if !deleted {
		return fmt.Errorf("statement #%d was not found", *id)
	}

	fmt.Fprintf(c.out, "%s Deleted statement #%d (%s %s) and its transactions.\n", successStyle.Render("OK"), statement.ID, statement.Date, emptyText(statement.Account, "no account"))
	return nil
}

func (c *CLI) runStatementSearch(ctx context.Context, store *storage.Store, args []string) error {
	fs := c.flagSet("statement search")
	id := fs.Int64("id", 0, "exact statement id")
	account := fs.String("account", "", "account search text")
	from := fs.String("from", "", "from date YYYY-MM-DD")
	to := fs.String("to", "", "to date YYYY-MM-DD")
	minBalance := fs.String("min-balance", "", "minimum balance")
	maxBalance := fs.String("max-balance", "", "maximum balance")
	limit := fs.Int("limit", 10, "maximum statements to match")
	txFlags := addTransactionModeFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	filter, err := buildStatementFilter(*id, *account, *from, *to, *minBalance, *maxBalance, *limit)
	if err != nil {
		return err
	}

	mode, err := parseTransactionMode(txFlags)
	if err != nil {
		return err
	}

	statements, err := store.SearchStatements(ctx, filter)
	if err != nil {
		return err
	}
	if len(statements) == 0 {
		fmt.Fprintln(c.out, "No statements matched.")
		return nil
	}

	statement := statements[0]
	transactions, err := store.Transactions(ctx, statement.ID, mode)
	if err != nil {
		return err
	}
	statement, err = store.StatementWithCalculatedRows(ctx, statement)
	if err != nil {
		return err
	}
	statements[0] = statement
	return c.renderStatement("Statement search", statements, statement, transactions, mode)
}

func (c *CLI) runStatementShow(ctx context.Context, store *storage.Store, args []string) error {
	positionalID, remainingArgs, err := leadingStatementID(args)
	if err != nil {
		return err
	}

	fs := c.flagSet("statement show")
	id := fs.Int64("id", positionalID, "statement id")
	txFlags := addTransactionModeFlags(fs)
	if err := fs.Parse(remainingArgs); err != nil {
		return err
	}

	if *id == 0 && fs.NArg() > 0 {
		parsed, err := strconv.ParseInt(fs.Arg(0), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid statement id %q", fs.Arg(0))
		}
		*id = parsed
	}
	if *id == 0 {
		return fmt.Errorf("statement id is required")
	}

	mode, err := parseTransactionMode(txFlags)
	if err != nil {
		return err
	}

	statement, ok, err := store.StatementByID(ctx, *id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("statement #%d was not found", *id)
	}
	transactions, err := store.Transactions(ctx, statement.ID, mode)
	if err != nil {
		return err
	}
	statement, err = store.StatementWithCalculatedRows(ctx, statement)
	if err != nil {
		return err
	}

	return c.renderStatement("Statement", []storage.Statement{statement}, statement, transactions, mode)
}

func (c *CLI) runTransactions(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) > 0 && args[0] == "load" {
		return c.runTransactionLoad(ctx, store, args[1:])
	}
	return c.runTransactionList(ctx, store, args)
}

func (c *CLI) runTransactionLoad(ctx context.Context, store *storage.Store, args []string) error {
	fs := c.flagSet("statement tx load")
	statementID := fs.Int64("statement-id", 0, "statement id")
	idAlias := fs.Int64("id", 0, "statement id")
	file := fs.String("file", "", "CSV file with transactions")
	date := fs.String("date", "", "transaction date YYYY-MM-DD")
	bucket := fs.String("bucket", "", "transaction bucket: income, cost, or savings")
	category := fs.String("category", "", "transaction category")
	description := fs.String("desc", "", "transaction description")
	descriptionAlias := fs.String("description", "", "transaction description")
	amount := fs.String("amount", "", "transaction amount")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *statementID == 0 {
		*statementID = *idAlias
	}
	if *statementID == 0 && fs.NArg() > 0 {
		parsed, err := strconv.ParseInt(fs.Arg(0), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid statement id %q", fs.Arg(0))
		}
		*statementID = parsed
	}
	if *file != "" {
		transactions, err := storage.LoadTransactionsCSV(*file, *statementID)
		if err != nil {
			return err
		}
		if len(transactions) == 0 {
			return fmt.Errorf("no transactions found in %s", *file)
		}
		created, err := c.ensureTransactionStatements(ctx, store, transactions, *statementID)
		if err != nil {
			return err
		}
		if err := store.InsertTransactions(ctx, transactions); err != nil {
			return err
		}
		fmt.Fprintf(c.out, "%s Loaded %d transaction(s) across %d statement(s)", successStyle.Render("OK"), len(transactions), countTransactionStatements(transactions))
		if created > 0 {
			fmt.Fprintf(c.out, "; created %d statement(s)", created)
		}
		fmt.Fprintln(c.out, ".")
		return nil
	}

	if *amount == "" {
		return fmt.Errorf("--amount is required when loading a single transaction")
	}
	transactionDate, err := storage.NormalizeDate(*date)
	if err != nil {
		return err
	}
	if transactionDate == "" {
		transactionDate = storage.Today()
	}
	amountCents, err := storage.ParseMoney(*amount)
	if err != nil {
		return err
	}
	normalizedBucket, err := storage.NormalizeBucket(*bucket)
	if err != nil {
		return err
	}
	if normalizedBucket == "" {
		return fmt.Errorf("--bucket is required; use income, cost, or savings")
	}
	normalizedCategory, err := storage.NormalizeCategory(*category)
	if err != nil {
		return err
	}
	if normalizedCategory == "" {
		return fmt.Errorf("--category is required; use one of: %s", storage.CategoriesText())
	}
	if *description == "" {
		*description = *descriptionAlias
	}
	statementIDForDate, created, err := c.ensureTransactionStatement(ctx, store, transactionDate, *statementID)
	if err != nil {
		return err
	}

	transaction := storage.Transaction{
		StatementID: statementIDForDate,
		Date:        transactionDate,
		Bucket:      normalizedBucket,
		Category:    normalizedCategory,
		Description: *description,
		AmountCents: amountCents,
	}
	if err := store.InsertTransactions(ctx, []storage.Transaction{transaction}); err != nil {
		return err
	}
	fmt.Fprintf(c.out, "%s Loaded transaction for statement #%d", successStyle.Render("OK"), statementIDForDate)
	if created {
		fmt.Fprint(c.out, " (created statement)")
	}
	fmt.Fprintln(c.out, ".")
	return nil
}

func (c *CLI) runTransactionList(ctx context.Context, store *storage.Store, args []string) error {
	positionalID, remainingArgs, err := leadingStatementID(args)
	if err != nil {
		return err
	}

	fs := c.flagSet("statement tx")
	statementID := fs.Int64("statement-id", positionalID, "statement id")
	idAlias := fs.Int64("id", 0, "statement id")
	bucket := fs.String("bucket", "", "filter by transaction bucket: income, cost, or savings")
	category := fs.String("category", "", "filter by transaction category")
	txFlags := addTransactionModeFlags(fs)
	if err := fs.Parse(remainingArgs); err != nil {
		return err
	}
	if *statementID == 0 {
		*statementID = *idAlias
	}
	if *statementID == 0 && fs.NArg() > 0 {
		parsed, err := strconv.ParseInt(fs.Arg(0), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid statement id %q", fs.Arg(0))
		}
		*statementID = parsed
	}
	if *statementID == 0 {
		return fmt.Errorf("statement id is required")
	}

	mode, err := parseTransactionMode(txFlags)
	if err != nil {
		return err
	}
	filter := storage.TransactionFilter{
		Bucket:   *bucket,
		Category: *category,
	}

	statement, ok, err := store.StatementByID(ctx, *statementID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("statement #%d was not found", *statementID)
	}
	transactions, err := store.TransactionsFiltered(ctx, *statementID, mode, filter)
	if err != nil {
		return err
	}
	statement, err = store.StatementWithCalculatedRows(ctx, statement)
	if err != nil {
		return err
	}

	return c.renderStatement("Statement transactions", []storage.Statement{statement}, statement, transactions, mode)
}

func (c *CLI) ensureTransactionStatements(ctx context.Context, store *storage.Store, transactions []storage.Transaction, requestedStatementID int64) (int, error) {
	created := 0
	seen := map[int64]bool{}
	for i := range transactions {
		statementID, wasCreated, err := c.ensureTransactionStatement(ctx, store, transactions[i].Date, requestedStatementID)
		if err != nil {
			return 0, err
		}
		transactions[i].StatementID = statementID
		if wasCreated && !seen[statementID] {
			created++
			seen[statementID] = true
		}
	}
	return created, nil
}

func (c *CLI) ensureTransactionStatement(ctx context.Context, store *storage.Store, transactionDate string, requestedStatementID int64) (int64, bool, error) {
	statementID, err := storage.StatementPeriodID(transactionDate)
	if err != nil {
		return 0, false, err
	}
	if requestedStatementID != 0 && requestedStatementID != statementID {
		_, _, period, err := storage.StatementPeriodRange(transactionDate)
		if err != nil {
			return 0, false, err
		}
		return 0, false, fmt.Errorf("statement #%d does not match transaction date %s; expected statement #%d for statement period %s", requestedStatementID, transactionDate, statementID, period)
	}

	existing, ok, err := store.StatementForStatementPeriod(ctx, transactionDate)
	if err != nil {
		return 0, false, err
	}
	if ok {
		return existing.ID, false, nil
	}

	if _, err := store.InsertStatement(ctx, storage.Statement{Date: transactionDate}); err != nil {
		return 0, false, err
	}
	return statementID, true, nil
}

func countTransactionStatements(transactions []storage.Transaction) int {
	seen := map[int64]bool{}
	for _, transaction := range transactions {
		seen[transaction.StatementID] = true
	}
	return len(seen)
}

func (c *CLI) calculatedStatements(ctx context.Context, store *storage.Store, statements []storage.Statement) ([]storage.Statement, error) {
	calculated := make([]storage.Statement, len(statements))
	for i, statement := range statements {
		next, err := store.StatementWithCalculatedRows(ctx, statement)
		if err != nil {
			return nil, err
		}
		calculated[i] = next
	}
	return calculated, nil
}

func (c *CLI) renderStatementList(title string, statements []storage.Statement) error {
	if !isTerminal(c.out) {
		fmt.Fprintln(c.out, ui.StaticStatementListView(title, statements))
		return nil
	}

	program := tea.NewProgram(ui.NewStatementListModel(title, statements))
	_, err := program.Run()
	return err
}

func (c *CLI) renderStatement(title string, statements []storage.Statement, statement storage.Statement, transactions []storage.Transaction, mode storage.TransactionMode) error {
	if !isTerminal(c.out) {
		fmt.Fprintln(c.out, ui.StaticStatementView(title, statements, statement, transactions, mode))
		return nil
	}

	program := tea.NewProgram(ui.NewStatementModel(title, statements, statement, transactions, mode))
	_, err := program.Run()
	return err
}

func (c *CLI) flagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(c.err)
	return fs
}

func (c *CLI) printUsage() {
	fmt.Fprint(c.out, usage())
}

func (c *CLI) printStatementUsage() {
	fmt.Fprint(c.out, statementUsage())
}

type transactionModeFlags struct {
	all          *bool
	top10        *bool
	top100       *bool
	currentMonth *bool
}

func addTransactionModeFlags(fs *flag.FlagSet) transactionModeFlags {
	return transactionModeFlags{
		all:          fs.Bool("all", false, "show all transactions"),
		top10:        fs.Bool("top-10", false, "show latest 10 transactions"),
		top100:       fs.Bool("top-100", false, "show latest 100 transactions"),
		currentMonth: fs.Bool("current-month", false, "show transactions from the current month"),
	}
}

func parseTransactionMode(flags transactionModeFlags) (storage.TransactionMode, error) {
	selected := []storage.TransactionMode{}
	if *flags.all {
		selected = append(selected, storage.TransactionAll)
	}
	if *flags.top10 {
		selected = append(selected, storage.TransactionTop10)
	}
	if *flags.top100 {
		selected = append(selected, storage.TransactionTop100)
	}
	if *flags.currentMonth {
		selected = append(selected, storage.TransactionCurrentMonth)
	}
	if len(selected) > 1 {
		return "", fmt.Errorf("transaction flags are mutually exclusive: use only one of --all, --top-10, --top-100, --current-month")
	}
	if len(selected) == 0 {
		return storage.TransactionTop10, nil
	}
	return selected[0], nil
}

func buildStatementFilter(id int64, account string, from string, to string, minBalance string, maxBalance string, limit int) (storage.StatementFilter, error) {
	filter := storage.StatementFilter{
		ID:              id,
		AccountContains: account,
		Limit:           limit,
	}

	var err error
	filter.FromDate, err = storage.NormalizeDate(from)
	if err != nil {
		return storage.StatementFilter{}, err
	}
	filter.ToDate, err = storage.NormalizeDate(to)
	if err != nil {
		return storage.StatementFilter{}, err
	}
	if minBalance != "" {
		value, err := storage.ParseMoney(minBalance)
		if err != nil {
			return storage.StatementFilter{}, err
		}
		filter.MinBalanceCents = &value
	}
	if maxBalance != "" {
		value, err := storage.ParseMoney(maxBalance)
		if err != nil {
			return storage.StatementFilter{}, err
		}
		filter.MaxBalanceCents = &value
	}

	return filter, nil
}

func ensureStatementLoadPeriodsAvailable(ctx context.Context, store *storage.Store, statements []storage.Statement) error {
	seen := map[string]int{}
	for i, statement := range statements {
		date := statement.Date
		if strings.TrimSpace(date) == "" {
			date = storage.Today()
		}

		_, _, period, err := storage.StatementPeriodRange(date)
		if err != nil {
			return err
		}
		if previous, ok := seen[period]; ok {
			return fmt.Errorf("statement load includes multiple statements for statement period %s at rows %d and %d", period, previous+1, i+1)
		}

		existing, ok, err := store.StatementForStatementPeriod(ctx, date)
		if err != nil {
			return err
		}
		if ok {
			return fmt.Errorf("statement #%d already exists for statement period %s (%s); delete it before loading another statement for that period", existing.ID, period, existing.Date)
		}

		seen[period] = i
	}
	return nil
}

func leadingStatementID(args []string) (int64, []string, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return 0, args, nil
	}

	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("invalid statement id %q", args[0])
	}
	return id, args[1:], nil
}

func emptyText(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func visitedFlags(fs *flag.FlagSet) map[string]bool {
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	return visited
}

func isTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	stat, err := file.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeCharDevice != 0
}

func usage() string {
	return strings.TrimSpace(`
budgeter [--db path] statement <command>

Commands:
  statement load              Load one statement or a CSV of statements.
  statement list              List statements.
  statement delete            Delete one statement and its transactions.
  statement search            Search statements and show the latest match with transactions.
  statement show              Show one statement by id.
  statement tx                Show transactions for a statement.
  statement tx load           Load one transaction or a CSV of transactions.

Examples:
  budgeter statement load --date 2026-08-20 --account cheque --balance 12000.00
  budgeter statement tx load --date 2026-08-23 --bucket cost --category Groceries --desc "Market" --amount -42.10
  budgeter statement list --limit 25
  budgeter statement delete --id 2607 --yes
  budgeter statement search --account cheque --from 2026-08-01 --top-10
  budgeter statement tx --statement-id 2607 --bucket cost --category Groceries --all
`) + "\n"
}

func statementUsage() string {
	return strings.TrimSpace(`
budgeter [--db path] statement <command>

Statement commands:
  load              Load one statement or CSV rows.
  list              List statements with --id, --account, --from, --to, --min-balance, --max-balance, --limit.
  delete            Delete one statement and its transactions with --id and --yes.
  search            Search statements with --id, --account, --from, --to, --min-balance, --max-balance, --limit.
  show              Show a statement by --id.
  tx                Show transaction rows for a statement. Supports --bucket and --category filters.
  tx load           Load transactions. Missing statements are created from transaction dates.

Statement load rejects a new statement when one already exists for the statement period. Statement periods run from the 21st through the 20th of the following month.
Transaction loads use each transaction date to find or create the matching statement.

Transaction read flags:
  --all
  --top-10
  --top-100
  --current-month

Transaction list filters:
  --bucket income|cost|savings
  --category category-name

CSV statements headers:
  date, account, balance, income, costs, earnings, savings, notes

CSV transaction headers:
  date, bucket, category, desc, amount

Transaction buckets:
  income
  cost
  savings

Transaction categories:
  Bills, Utilities, Groceries, Eating Out, Subscriptions, Transport, Shopping, Entertainment, Debt Repayments, Investments, Insurance, Healthcare, Personal Care, Miscellaneous
`) + "\n"
}
