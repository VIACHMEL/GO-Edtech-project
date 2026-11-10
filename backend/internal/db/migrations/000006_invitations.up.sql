-- Доступ по одномуразовому приглашению. Храним только SHA-256 хеш токена.
CREATE TABLE invitations (
    id BIGSERIAL PRIMARY KEY,
    teacher_id BIGINT NOT NULL REFERENCES users(id),
    group_id BIGINT REFERENCES groups(id),
    invitee_email TEXT,
    token_hash TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_by BIGINT REFERENCES users(id),
    accepted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (status IN ('pending', 'accepted', 'revoked')),
    CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    CHECK (invitee_email IS NULL OR (invitee_email = lower(btrim(invitee_email)) AND length(invitee_email) > 0)),
    CHECK (
        (status = 'accepted' AND accepted_by IS NOT NULL AND accepted_at IS NOT NULL)
        OR (status <> 'accepted' AND accepted_by IS NULL AND accepted_at IS NULL)
    )
);

CREATE INDEX idx_invitations_teacher ON invitations(teacher_id);
CREATE INDEX idx_invitations_group ON invitations(group_id) WHERE group_id IS NOT NULL;
