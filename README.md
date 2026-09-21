# xpense tracker 

`xt` is a Go CLI for loading statements, loading transactions for a statement, and searching statements with a Bubble Tea terminal view.

## Build

```sh
make
```

To install the executable to `~/bin`:

```sh
make install
```

## Commands

```sh
xt [--db path] statement load
xt [--db path] statement list
xt [--db path] statement delete
xt [--db path] statement tx load
xt [--db path] statement search
xt [--db path] statement show
xt [--db path] statement tx
```

The default database path is `~/.xt/xt.db`.

## Examples

```sh
xt statement load --date 2026-08-20 --account cheque --balance 12000.00 --income 5000.00 --costs 2000.00 --earnings 3000.00 --savings 1000.00
xt statement tx load --date 2026-08-23 --bucket cost --category Groceries --description "Market" --amount -42.10
xt statement list --limit 25
xt statement delete --id 2607 --yes
xt statement search --account cheque --from 2026-08-01 --top-10
xt statement tx --statement-id 2607 --bucket cost --category Groceries --all
```

Statement loads use the latest saved statement as defaults. If you load a new statement and omit `account`, `income`, `costs`, `earnings`, or `savings`, those values are copied from the latest statement in the database. Statement IDs use `YYMM` from the statement period start date, so the `2026-08-21` through `2026-09-20` statement is `2608`. A load is rejected when a statement already exists for that statement period. Statement periods run from the 21st through the 20th of the following month, so `2026-08-23` belongs to `2026-08-21` through `2026-09-20`.

Transaction loads use each transaction date to find the matching statement. If that statement does not exist yet, it is created automatically.

Statement deletes require `--yes` and remove the statement plus its transactions.

## CSV

Statement CSV headers:

```csv
date,account,balance,income,costs,earnings,savings,notes
```

Transaction CSV headers:

```csv
date,bucket,category,desc,amount
```

Sample transaction CSV:

```csv
date,bucket,category,desc,amount
2026-08-01,income,Miscellaneous,Monthly salary,5000.00
2026-08-02,cost,Groceries,Market,-42.10
2026-08-03,cost,Transport,Fuel,-65.50
2026-08-04,savings,Investments,Savings transfer,-500.00
```

Load it with:

```sh
xt statement tx load --file txs.csv
```

Transaction buckets are `income`, `cost`, and `savings`. Bucketed transactions are used to calculate statement `income`, `costs`, `earnings`, `savings`, and `balance` when a statement is shown or listed. `earnings` is `income - costs`; `balance` is `income - costs - savings`. Existing unbucketed transactions are still displayed, but they are not included in calculated statement rows.

Transaction categories are `Bills`, `Utilities`, `Groceries`, `Eating Out`, `Subscriptions`, `Transport`, `Shopping`, `Entertainment`, `Debt Repayments`, `Investments`, `Insurance`, `Healthcare`, `Personal Care`, and `Miscellaneous`.

Transaction lists can be filtered by bucket and category:

```sh
xt statement tx --statement-id 2607 --bucket cost --category Groceries --all
```

Transaction read flags are mutually exclusive:

```sh
--all
--top-10
--top-100
--current-month
```
