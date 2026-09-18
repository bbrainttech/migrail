## migrail

**4 errors, 4 warnings, 0 notices** in 4 migrations. 4 findings at or above `error` fail this check.

| Severity | Rule | Location | Finding |
| --- | --- | --- | --- |
| error | MR101 | `../../testdata/sql/MR101/bad/basic.sql:1` | Index creation blocks writes on "orders" |
| warning | MR403 | `../../testdata/sql/MR101/bad/basic.sql:1` | No lock_timeout before locking DDL |
| error | MR201 | `../../testdata/sql/MR201/bad/two_columns.sql:1` | Changing the type of "orders.user_id" rewrites the table |
| error | MR201 | `../../testdata/sql/MR201/bad/two_columns.sql:1` | Changing the type of "orders.external_id" rewrites the table |
| warning | MR403 | `../../testdata/sql/MR201/bad/two_columns.sql:1` | No lock_timeout before locking DDL |
| warning | MR403 | `../../testdata/sql/MR302/bad/in_transaction.sql:2` | No lock_timeout before locking DDL |
| error | MR302 | `../../testdata/sql/MR302/bad/in_transaction.sql:3` | Renaming column "users.users" breaks running code |
| warning | MR403 | `../../testdata/sql/MR904/good/with_reason.sql:2` | No lock_timeout before locking DDL |

<details>
<summary><b>error</b> MR101 · Index creation blocks writes on "orders"</summary>

`../../testdata/sql/MR101/bad/basic.sql:1`

```sql
CREATE INDEX idx_orders_status ON orders (status)
```

**Lock:** SHARE on orders · blocks INSERT, UPDATE, DELETE

**Why:** Postgres holds this lock until the whole index is built. INSERT, UPDATE and DELETE on the table wait for the entire build.

**Fix:** Build the index concurrently, outside a transaction:

```sql
CREATE INDEX CONCURRENTLY idx_orders_status ON orders (status);
```

Run `migrail explain MR101` for details.

</details>

<details>
<summary><b>warning</b> MR403 · No lock_timeout before locking DDL</summary>

`../../testdata/sql/MR101/bad/basic.sql:1`

```sql
CREATE INDEX idx_orders_status ON orders (status)
```

**Why:** If a long query holds a lock on orders, this statement waits for it, and every query that arrives afterwards queues behind this statement.

**Fix:** Set a lock timeout at the top of the migration, and retry the migration if it times out:

```sql
SET lock_timeout = '5s';
```

Run `migrail explain MR403` for details.

</details>

<details>
<summary><b>error</b> MR201 · Changing the type of "orders.user_id" rewrites the table</summary>

`../../testdata/sql/MR201/bad/two_columns.sql:1`

```sql
ALTER TABLE orders
  ALTER COLUMN user_id TYPE bigint,
  ALTER COLUMN external_id TYPE uuid USING external_id::uuid
```

**Lock:** ACCESS EXCLUSIVE on orders · blocks reads and writes · rewrites the table

**Why:** Postgres rewrites every row of orders and rebuilds its indexes while it holds an ACCESS EXCLUSIVE lock. Reads and writes wait until the rewrite finishes.

**Fix:** Change the type with expand and contract instead of rewriting the table in place:

1. Add user_id_new with the new type

```sql
ALTER TABLE orders ADD COLUMN user_id_new bigint;
```

2. Write to both columns and backfill user_id_new in batches

3. Read from user_id_new, then drop user_id and rename user_id_new

Run `migrail explain MR201` for details.

</details>

<details>
<summary><b>error</b> MR201 · Changing the type of "orders.external_id" rewrites the table</summary>

`../../testdata/sql/MR201/bad/two_columns.sql:1`

```sql
ALTER TABLE orders
  ALTER COLUMN user_id TYPE bigint,
  ALTER COLUMN external_id TYPE uuid USING external_id::uuid
```

**Lock:** ACCESS EXCLUSIVE on orders · blocks reads and writes · rewrites the table

**Why:** Postgres rewrites every row of orders and rebuilds its indexes while it holds an ACCESS EXCLUSIVE lock. Reads and writes wait until the rewrite finishes.

**Fix:** Change the type with expand and contract instead of rewriting the table in place:

1. Add external_id_new with the new type

```sql
ALTER TABLE orders ADD COLUMN external_id_new uuid;
```

2. Write to both columns and backfill external_id_new in batches

3. Read from external_id_new, then drop external_id and rename external_id_new

Run `migrail explain MR201` for details.

</details>

<details>
<summary><b>warning</b> MR403 · No lock_timeout before locking DDL</summary>

`../../testdata/sql/MR201/bad/two_columns.sql:1`

```sql
ALTER TABLE orders
  ALTER COLUMN user_id TYPE bigint,
  ALTER COLUMN external_id TYPE uuid USING external_id::uuid
```

**Why:** If a long query holds a lock on orders, this statement waits for it, and every query that arrives afterwards queues behind this statement.

**Fix:** Set a lock timeout at the top of the migration, and retry the migration if it times out:

```sql
SET lock_timeout = '5s';
```

Run `migrail explain MR403` for details.

</details>

<details>
<summary><b>warning</b> MR403 · No lock_timeout before locking DDL</summary>

`../../testdata/sql/MR302/bad/in_transaction.sql:2`

```sql
ALTER TABLE users ADD COLUMN nickname text
```

**Why:** If a long query holds a lock on users, this statement waits for it, and every query that arrives afterwards queues behind this statement.

**Fix:** Set a lock timeout at the top of the migration, and retry the migration if it times out:

```sql
SET lock_timeout = '5s';
```

Run `migrail explain MR403` for details.

</details>

<details>
<summary><b>error</b> MR302 · Renaming column "users.users" breaks running code</summary>

`../../testdata/sql/MR302/bad/in_transaction.sql:3`

```sql
ALTER TABLE users RENAME COLUMN users TO username
```

**Lock:** ACCESS EXCLUSIVE on users · blocks reads and writes

**Why:** During a rolling deploy, instances running the previous release still query "users". Those queries fail as soon as this migration runs.

**Fix:** Rename in three deploys with expand and contract:

1. Add username, write to both columns and backfill it

2. Read from username and stop writing users

3. Drop users

Run `migrail explain MR302` for details.

</details>

<details>
<summary><b>warning</b> MR403 · No lock_timeout before locking DDL</summary>

`../../testdata/sql/MR904/good/with_reason.sql:2`

```sql
CREATE INDEX idx_orders_status ON orders (status)
```

**Why:** If a long query holds a lock on orders, this statement waits for it, and every query that arrives afterwards queues behind this statement.

**Fix:** Set a lock timeout at the top of the migration, and retry the migration if it times out:

```sql
SET lock_timeout = '5s';
```

Run `migrail explain MR403` for details.

</details>

<sub>migrail dev · PostgreSQL 16 · 7 statements · 1 ignored</sub>
