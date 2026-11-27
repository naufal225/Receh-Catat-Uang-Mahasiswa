-- Finance module tables. Applied after the core schema (they reference users).
-- Statements are split on ";" at the end of a line; keep semicolons out of comments.

-- MySQL has no partial index. active_name is NULL once a wallet is archived, and
-- a unique index ignores NULLs, so only live wallets need distinct names.
CREATE TABLE IF NOT EXISTS wallets (
    id          BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
    org_id     BIGINT      NOT NULL,
    name        VARCHAR(40) NOT NULL,
    type        VARCHAR(10) NOT NULL,
    color       VARCHAR(8)  NOT NULL,
    balance     BIGINT      NOT NULL DEFAULT 0,
    archived    TINYINT(1)  NOT NULL DEFAULT 0,
    created_at  BIGINT      NOT NULL,
    active_name VARCHAR(40) GENERATED ALWAYS AS (IF(archived = 0, name, NULL)) STORED,
    UNIQUE KEY uq_wallet_name (org_id, active_name),
    CONSTRAINT fk_wallets_org FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT ck_wallets_type CHECK (type IN ('bank', 'ewallet', 'cash', 'other')),
    CONSTRAINT ck_wallets_balance CHECK (balance >= 0)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS categories (
    id      BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
    org_id BIGINT      NOT NULL,
    name    VARCHAR(40) NOT NULL,
    type    VARCHAR(10) NOT NULL,
    color   VARCHAR(8)  NOT NULL,
    UNIQUE KEY uq_category_name (org_id, type, name),
    CONSTRAINT fk_categories_org FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT ck_categories_type CHECK (type IN ('income', 'expense'))
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS recurring (
    id          BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
    org_id     BIGINT      NOT NULL,
    wallet_id   BIGINT      NOT NULL,
    category_id BIGINT      NOT NULL,
    type        VARCHAR(10) NOT NULL,
    name        VARCHAR(80) NOT NULL,
    amount      BIGINT      NOT NULL,
    frequency   VARCHAR(10) NOT NULL,
    anchor_day  INT         NOT NULL,
    next_due    DATE        NOT NULL,
    active      TINYINT(1)  NOT NULL DEFAULT 1,
    last_error  VARCHAR(300) NOT NULL DEFAULT '',
    KEY idx_recurring_due (active, next_due),
    CONSTRAINT fk_recurring_org FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT fk_recurring_wallet FOREIGN KEY (wallet_id) REFERENCES wallets (id),
    CONSTRAINT fk_recurring_category FOREIGN KEY (category_id) REFERENCES categories (id),
    CONSTRAINT ck_recurring_type CHECK (type IN ('income', 'expense')),
    CONSTRAINT ck_recurring_amount CHECK (amount > 0),
    CONSTRAINT ck_recurring_frequency CHECK (frequency IN ('daily', 'weekly', 'monthly', 'yearly'))
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- A transfer moves money between two wallets of one user. amount is what the
-- destination receives, fee is paid on top by the source.
CREATE TABLE IF NOT EXISTS transfers (
    id             BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
    org_id        BIGINT      NOT NULL,
    from_wallet_id BIGINT      NOT NULL,
    to_wallet_id   BIGINT      NOT NULL,
    amount         BIGINT      NOT NULL,
    fee            BIGINT      NOT NULL DEFAULT 0,
    note           VARCHAR(80) NOT NULL DEFAULT '',
    occurred_on    DATE        NOT NULL,
    created_at     BIGINT      NOT NULL,
    KEY idx_transfers_route (org_id, from_wallet_id, to_wallet_id, id),
    CONSTRAINT fk_transfers_org FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT fk_transfers_from FOREIGN KEY (from_wallet_id) REFERENCES wallets (id),
    CONSTRAINT fk_transfers_to FOREIGN KEY (to_wallet_id) REFERENCES wallets (id),
    CONSTRAINT ck_transfers_amount CHECK (amount > 0),
    CONSTRAINT ck_transfers_fee CHECK (fee >= 0),
    CONSTRAINT ck_transfers_wallets CHECK (from_wallet_id <> to_wallet_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Every balance change is a row here. A transfer writes a transfer_out leg on
-- the source, a transfer_in leg on the destination and, if it has a fee, an
-- ordinary expense in the "Biaya admin" category. All three carry transfer_id.
CREATE TABLE IF NOT EXISTS transactions (
    id           BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
    org_id      BIGINT      NOT NULL,
    wallet_id    BIGINT      NOT NULL,
    category_id  BIGINT      NULL,
    type         VARCHAR(12) NOT NULL,
    name         VARCHAR(80) NOT NULL,
    amount       BIGINT      NOT NULL,
    occurred_on  DATE        NOT NULL,
    recurring_id BIGINT      NULL,
    transfer_id  BIGINT      NULL,
    import_batch_id BIGINT   NULL,
    created_at   BIGINT      NOT NULL,
    KEY idx_tx_org_date (org_id, occurred_on DESC, id DESC),
    KEY idx_tx_wallet (wallet_id),
    KEY idx_tx_category (category_id),
    KEY idx_tx_transfer (transfer_id),
    KEY idx_tx_import (import_batch_id),
    -- A rule posts at most once per date, so a restart or retry never double
    -- charges. Rows without a rule have a NULL recurring_id, which never collides.
    UNIQUE KEY uq_tx_occurrence (recurring_id, occurred_on),
    CONSTRAINT fk_tx_org FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT fk_tx_wallet FOREIGN KEY (wallet_id) REFERENCES wallets (id),
    CONSTRAINT fk_tx_category FOREIGN KEY (category_id) REFERENCES categories (id),
    CONSTRAINT fk_tx_recurring FOREIGN KEY (recurring_id) REFERENCES recurring (id) ON DELETE SET NULL,
    CONSTRAINT fk_tx_transfer FOREIGN KEY (transfer_id) REFERENCES transfers (id),
    CONSTRAINT ck_tx_type CHECK (type IN ('income', 'expense', 'transfer_out', 'transfer_in')),
    CONSTRAINT ck_tx_amount CHECK (amount > 0)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Monthly spending cap per expense category. Deleting the category deletes
-- its budget: a budget for a category that no longer exists means nothing.
CREATE TABLE IF NOT EXISTS budgets (
    id          BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    org_id     BIGINT NOT NULL,
    category_id BIGINT NOT NULL,
    amount      BIGINT NOT NULL,
    created_at  BIGINT NOT NULL,
    UNIQUE KEY uq_budget_category (org_id, category_id),
    CONSTRAINT fk_budget_org FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT fk_budget_category FOREIGN KEY (category_id) REFERENCES categories (id) ON DELETE CASCADE,
    CONSTRAINT ck_budget_amount CHECK (amount > 0)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Savings goals. Progress is the linked wallet's balance when wallet_id is set,
-- otherwise the sum of goal_contributions (a virtual piggy bank).
CREATE TABLE IF NOT EXISTS goals (
    id            BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
    org_id       BIGINT      NOT NULL,
    name          VARCHAR(60) NOT NULL,
    target_amount BIGINT      NOT NULL,
    deadline      DATE        NULL,
    wallet_id     BIGINT      NULL,
    color         VARCHAR(8)  NOT NULL,
    archived_at   BIGINT      NULL,
    created_at    BIGINT      NOT NULL,
    KEY idx_goals_org (org_id, archived_at),
    CONSTRAINT fk_goal_org FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT fk_goal_wallet FOREIGN KEY (wallet_id) REFERENCES wallets (id) ON DELETE SET NULL,
    CONSTRAINT ck_goal_target CHECK (target_amount > 0)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS goal_contributions (
    id          BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
    goal_id     BIGINT      NOT NULL,
    amount      BIGINT      NOT NULL,
    note        VARCHAR(80) NOT NULL DEFAULT '',
    occurred_on DATE        NOT NULL,
    created_at  BIGINT      NOT NULL,
    KEY idx_contrib_goal (goal_id, occurred_on),
    CONSTRAINT fk_contrib_goal FOREIGN KEY (goal_id) REFERENCES goals (id) ON DELETE CASCADE,
    CONSTRAINT ck_contrib_amount CHECK (amount <> 0)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- One imported statement. Its transactions carry import_batch_id so the whole
-- import can be undone at once.
CREATE TABLE IF NOT EXISTS import_batches (
    id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    org_id    BIGINT       NOT NULL,
    wallet_id  BIGINT       NOT NULL,
    file_name  VARCHAR(120) NOT NULL DEFAULT '',
    row_count  INT          NOT NULL,
    total_in   BIGINT       NOT NULL,
    total_out  BIGINT       NOT NULL,
    created_at BIGINT       NOT NULL,
    KEY idx_import_org (org_id, created_at),
    CONSTRAINT fk_import_org FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT fk_import_wallet FOREIGN KEY (wallet_id) REFERENCES wallets (id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;
