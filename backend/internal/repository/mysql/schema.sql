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

-- Invitation / promo codes that grant premium for a while.
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

-- Premium periods. A row never disappears: revoking sets revoked_at, so the
-- admin can always see who had premium, from where and until when.
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

-- In-app notifications. ref_key names the event, unique per user, so a sweep
-- that runs twice (or after a restart) never notifies twice.
CREATE TABLE IF NOT EXISTS notifications (
    id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id    BIGINT       NOT NULL,
    kind       VARCHAR(20)  NOT NULL,
    title      VARCHAR(120) NOT NULL,
    body       VARCHAR(300) NOT NULL DEFAULT '',
    link       VARCHAR(120) NOT NULL DEFAULT '',
    ref_key    VARCHAR(80)  NOT NULL,
    read_at    BIGINT       NULL,
    created_at BIGINT       NOT NULL,
    UNIQUE KEY uq_notification_ref (user_id, ref_key),
    KEY idx_notifications_inbox (user_id, created_at DESC),
    KEY idx_notifications_age (created_at),
    CONSTRAINT fk_notif_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS notification_prefs (
    user_id               BIGINT       NOT NULL PRIMARY KEY,
    disabled_kinds        VARCHAR(300) NOT NULL DEFAULT '',
    -- Module settings as a JSON object, e.g. {"finance.reminder_hour": 20}.
    settings              TEXT         NULL,
    CONSTRAINT fk_nprefs_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;


-- Organizations (workspaces). Module data belongs to an org, not to a user.
-- owner_id is the one person who pays and may delete the org.
CREATE TABLE IF NOT EXISTS organizations (
    id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name       VARCHAR(80)  NOT NULL,
    personal   TINYINT(1)   NOT NULL DEFAULT 0,
    owner_id   BIGINT       NOT NULL,
    created_at BIGINT       NOT NULL,
    KEY idx_org_owner (owner_id),
    CONSTRAINT fk_org_owner FOREIGN KEY (owner_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS org_members (
    org_id     BIGINT      NOT NULL,
    user_id    BIGINT      NOT NULL,
    role       VARCHAR(8)  NOT NULL,
    created_at BIGINT      NOT NULL,
    PRIMARY KEY (org_id, user_id),
    KEY idx_org_members_user (user_id),
    CONSTRAINT fk_orgm_org FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT fk_orgm_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT ck_orgm_role CHECK (role IN ('owner', 'admin', 'member'))
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Invitations by email. Only the SHA-256 of the token is kept.
CREATE TABLE IF NOT EXISTS org_invitations (
    id          BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    org_id      BIGINT       NOT NULL,
    email       VARCHAR(254) NOT NULL,
    role        VARCHAR(8)   NOT NULL,
    token_hash  CHAR(64)     NOT NULL,
    invited_by  BIGINT       NULL,
    expires_at  BIGINT       NOT NULL,
    accepted_at BIGINT       NULL,
    created_at  BIGINT       NOT NULL,
    UNIQUE KEY uq_invite_token (token_hash),
    KEY idx_invite_org (org_id, accepted_at),
    CONSTRAINT fk_invite_org FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT fk_invite_by FOREIGN KEY (invited_by) REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT ck_invite_role CHECK (role IN ('admin', 'member'))
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Billing. A payment is one checkout for one period of one workspace. The org
-- and payer are kept as a name/email snapshot too, so the books survive the
-- deletion of a workspace or an account.
CREATE TABLE IF NOT EXISTS payments (
    id           BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    org_id       BIGINT       NULL,
    org_name     VARCHAR(80)  NOT NULL DEFAULT '',
    payer_id     BIGINT       NULL,
    payer_email  VARCHAR(254) NOT NULL DEFAULT '',
    price_id     VARCHAR(32)  NOT NULL,
    amount       BIGINT       NOT NULL,
    currency     CHAR(3)      NOT NULL,
    provider     VARCHAR(16)  NOT NULL,
    provider_ref VARCHAR(100) NOT NULL DEFAULT '',
    external_id  VARCHAR(64)  NOT NULL,
    status       VARCHAR(8)   NOT NULL DEFAULT 'pending',
    checkout_url VARCHAR(500) NOT NULL DEFAULT '',
    period_start BIGINT       NULL,
    period_end   BIGINT       NULL,
    expires_at   BIGINT       NOT NULL,
    paid_at      BIGINT       NULL,
    created_at   BIGINT       NOT NULL,
    UNIQUE KEY uq_payment_external (external_id),
    KEY idx_payment_org (org_id, status),
    KEY idx_payment_status (status, created_at),
    CONSTRAINT fk_payment_org FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE SET NULL,
    CONSTRAINT fk_payment_payer FOREIGN KEY (payer_id) REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT ck_payment_status CHECK (status IN ('pending', 'paid', 'expired', 'failed')),
    CONSTRAINT ck_payment_amount CHECK (amount > 0)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- The paid state of a workspace: one row, extended by every payment.
CREATE TABLE IF NOT EXISTS subscriptions (
    org_id               BIGINT      NOT NULL PRIMARY KEY,
    price_id             VARCHAR(32) NOT NULL,
    period_end           BIGINT      NOT NULL,
    cancel_at_period_end TINYINT(1)  NOT NULL DEFAULT 0,
    state                VARCHAR(8)  NOT NULL DEFAULT 'active',
    last_payment_id      BIGINT      NULL,
    updated_at           BIGINT      NOT NULL,
    KEY idx_subscription_end (state, period_end),
    CONSTRAINT fk_subscription_org FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT fk_subscription_payment FOREIGN KEY (last_payment_id) REFERENCES payments (id) ON DELETE SET NULL
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Gateway notifications already handled; (provider, event_id) is applied once.
CREATE TABLE IF NOT EXISTS webhook_events (
    provider    VARCHAR(16) NOT NULL,
    event_id    VARCHAR(100) NOT NULL,
    received_at BIGINT      NOT NULL,
    PRIMARY KEY (provider, event_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;
