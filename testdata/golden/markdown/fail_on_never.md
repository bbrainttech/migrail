## migrail

**1 error, 1 warning, 0 notices** in 1 migration.

| Severity | Rule | Location | Finding |
| --- | --- | --- | --- |
| error | MR101 | `../../testdata/sql/MR101/bad/basic.sql:1` | Index creation blocks writes on "orders" |
| warning | MR403 | `../../testdata/sql/MR101/bad/basic.sql:1` | No lock_timeout before locking DDL |

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

<sub>migrail dev · PostgreSQL 16 · 1 statement</sub>
