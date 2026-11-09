-- Applied on every start; every statement is idempotent. Statements are split on
-- ";" at the end of a line, so keep semicolons out of comments.
-- Money is whole rupiah in BIGINT. Instants are unix seconds, calendar days are DATE.
-- utf8mb4_0900_ai_ci compares case-insensitively, so "BCA" and "bca" collide.

CREATE TABLE IF NOT EXISTS users (
    id            BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name          VARCHAR(80)  NOT NULL,
    email         VARCHAR(254) NOT NULL,
    -- Empty for accounts that only ever signed in with Google.
    password_hash VARCHAR(100) NOT NULL,
    google_sub    VARCHAR(64)  NULL,
    role          VARCHAR(10)  NOT NULL DEFAULT 'user',
    status        VARCHAR(10)  NOT NULL DEFAULT 'active',
    status_reason VARCHAR(300) NOT NULL DEFAULT '',
    created_at    BIGINT       NOT NULL,
    UNIQUE KEY uq_users_email (email),
    UNIQUE KEY uq_users_google (google_sub),
    CONSTRAINT ck_users_role CHECK (role IN ('user', 'admin')),
    CONSTRAINT ck_users_status CHECK (status IN ('active', 'suspended', 'frozen'))
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS sessions (
    id           BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id      BIGINT       NOT NULL,
    token_hash   CHAR(64)     NOT NULL,
    ip           VARCHAR(64)  NOT NULL,
    user_agent   VARCHAR(512) NOT NULL,
    device       VARCHAR(120) NOT NULL,
    location     VARCHAR(160) NOT NULL DEFAULT '',
    created_at   BIGINT       NOT NULL,
    last_seen_at BIGINT       NOT NULL,
    expires_at   BIGINT       NOT NULL,
    UNIQUE KEY uq_sessions_token (token_hash),
    KEY idx_sessions_user (user_id),
    CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- MySQL has no partial index. active_name is NULL once a wallet is archived, and
-- a unique index ignores NULLs, so only live wallets need distinct names.
CREATE TABLE IF NOT EXISTS wallets (
    id          BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id     BIGINT      NOT NULL,
    name        VARCHAR(40) NOT NULL,
    type        VARCHAR(10) NOT NULL,
    color       VARCHAR(8)  NOT NULL,
    balance     BIGINT      NOT NULL DEFAULT 0,
    archived    TINYINT(1)  NOT NULL DEFAULT 0,
    created_at  BIGINT      NOT NULL,
    active_name VARCHAR(40) GENERATED ALWAYS AS (IF(archived = 0, name, NULL)) STORED,
    UNIQUE KEY uq_wallet_name (user_id, active_name),
    CONSTRAINT fk_wallets_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT ck_wallets_type CHECK (type IN ('bank', 'ewallet', 'cash', 'other')),
    CONSTRAINT ck_wallets_balance CHECK (balance >= 0)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS categories (
    id      BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT      NOT NULL,
    name    VARCHAR(40) NOT NULL,
    type    VARCHAR(10) NOT NULL,
    color   VARCHAR(8)  NOT NULL,
    UNIQUE KEY uq_category_name (user_id, type, name),
    CONSTRAINT fk_categories_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT ck_categories_type CHECK (type IN ('income', 'expense'))
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS recurring (
    id          BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id     BIGINT      NOT NULL,
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
    CONSTRAINT fk_recurring_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
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
    user_id        BIGINT      NOT NULL,
    from_wallet_id BIGINT      NOT NULL,
    to_wallet_id   BIGINT      NOT NULL,
    amount         BIGINT      NOT NULL,
    fee            BIGINT      NOT NULL DEFAULT 0,
    note           VARCHAR(80) NOT NULL DEFAULT '',
    occurred_on    DATE        NOT NULL,
    created_at     BIGINT      NOT NULL,
    KEY idx_transfers_route (user_id, from_wallet_id, to_wallet_id, id),
    CONSTRAINT fk_transfers_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
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
    user_id      BIGINT      NOT NULL,
    wallet_id    BIGINT      NOT NULL,
    category_id  BIGINT      NULL,
    type         VARCHAR(12) NOT NULL,
    name         VARCHAR(80) NOT NULL,
    amount       BIGINT      NOT NULL,
    occurred_on  DATE        NOT NULL,
    recurring_id BIGINT      NULL,
    transfer_id  BIGINT      NULL,
    created_at   BIGINT      NOT NULL,
    KEY idx_tx_user_date (user_id, occurred_on DESC, id DESC),
    KEY idx_tx_wallet (wallet_id),
    KEY idx_tx_category (category_id),
    KEY idx_tx_transfer (transfer_id),
    -- A rule posts at most once per date, so a restart or retry never double
    -- charges. Rows without a rule have a NULL recurring_id, which never collides.
    UNIQUE KEY uq_tx_occurrence (recurring_id, occurred_on),
    CONSTRAINT fk_tx_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_tx_wallet FOREIGN KEY (wallet_id) REFERENCES wallets (id),
    CONSTRAINT fk_tx_category FOREIGN KEY (category_id) REFERENCES categories (id),
    CONSTRAINT fk_tx_recurring FOREIGN KEY (recurring_id) REFERENCES recurring (id) ON DELETE SET NULL,
    CONSTRAINT fk_tx_transfer FOREIGN KEY (transfer_id) REFERENCES transfers (id),
    CONSTRAINT ck_tx_type CHECK (type IN ('income', 'expense', 'transfer_out', 'transfer_in')),
    CONSTRAINT ck_tx_amount CHECK (amount > 0)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS audit_logs (
    id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id    BIGINT       NULL,
    email      VARCHAR(254) NOT NULL DEFAULT '',
    event      VARCHAR(40)  NOT NULL,
    ip         VARCHAR(64)  NOT NULL DEFAULT '',
    device     VARCHAR(120) NOT NULL DEFAULT '',
    location   VARCHAR(160) NOT NULL DEFAULT '',
    detail     VARCHAR(500) NOT NULL DEFAULT '',
    created_at BIGINT       NOT NULL,
    KEY idx_audit_time (created_at DESC, id DESC),
    CONSTRAINT fk_audit_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE SET NULL
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Premium periods. A row never disappears: revoking sets revoked_at, so the
-- admin can always see who had premium, from where and until when.
CREATE TABLE IF NOT EXISTS promo_codes (
    id            BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    code          VARCHAR(32)  NOT NULL,
    plan          VARCHAR(16)  NOT NULL DEFAULT 'premium',
    duration_days INT          NULL,
    max_uses      INT          NOT NULL,
    used          INT          NOT NULL DEFAULT 0,
    expires_at    BIGINT       NULL,
    note          VARCHAR(200) NOT NULL DEFAULT '',
    active        TINYINT(1)   NOT NULL DEFAULT 1,
    created_by    BIGINT       NULL,
    created_at    BIGINT       NOT NULL,
    UNIQUE KEY uq_promo_code (code),
    CONSTRAINT fk_promo_creator FOREIGN KEY (created_by) REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT ck_promo_plan CHECK (plan IN ('premium')),
    CONSTRAINT ck_promo_uses CHECK (max_uses > 0 AND used >= 0 AND used <= max_uses),
    CONSTRAINT ck_promo_duration CHECK (duration_days IS NULL OR duration_days > 0)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS entitlements (
    id            BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id       BIGINT       NOT NULL,
    source        VARCHAR(8)   NOT NULL,
    plan          VARCHAR(16)  NOT NULL DEFAULT 'premium',
    starts_at     BIGINT       NOT NULL,
    ends_at       BIGINT       NULL,
    note          VARCHAR(200) NOT NULL DEFAULT '',
    granted_by    BIGINT       NULL,
    promo_code_id BIGINT       NULL,
    revoked_at    BIGINT       NULL,
    created_at    BIGINT       NOT NULL,
    KEY idx_entitlements_user (user_id, revoked_at),
    CONSTRAINT fk_ent_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_ent_granter FOREIGN KEY (granted_by) REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT fk_ent_promo FOREIGN KEY (promo_code_id) REFERENCES promo_codes (id) ON DELETE SET NULL,
    CONSTRAINT ck_ent_source CHECK (source IN ('paid', 'comp', 'code')),
    CONSTRAINT ck_ent_plan CHECK (plan IN ('premium'))
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- One redemption per user per code, enforced by the database.
CREATE TABLE IF NOT EXISTS promo_redemptions (
    id          BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    code_id     BIGINT NOT NULL,
    user_id     BIGINT NOT NULL,
    redeemed_at BIGINT NOT NULL,
    UNIQUE KEY uq_redemption (code_id, user_id),
    CONSTRAINT fk_red_code FOREIGN KEY (code_id) REFERENCES promo_codes (id) ON DELETE CASCADE,
    CONSTRAINT fk_red_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;
