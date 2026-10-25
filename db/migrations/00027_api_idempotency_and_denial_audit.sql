-- +goose Up
-- P06 retains the existing idempotency response-reference column because older
-- records may use an earlier internal representation. New API commands write
-- the versioned v1ref format validated in application code.
CREATE TABLE authorization_denials (
    id uuid PRIMARY KEY,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    actor_subject varchar(256) NOT NULL,
    route varchar(256) NOT NULL,
    action varchar(128) NOT NULL,
    reason_code varchar(128) NOT NULL,
    correlation_id uuid NOT NULL,
    CONSTRAINT authorization_denials_actor_nonempty CHECK (length(actor_subject) > 0),
    CONSTRAINT authorization_denials_route_canonical CHECK (route ~ '^/[A-Za-z0-9._~!$&''()*+,;=:@/%-]*$'),
    CONSTRAINT authorization_denials_action_nonempty CHECK (length(action) > 0 AND length(reason_code) > 0)
);
CREATE INDEX authorization_denials_occurred_at_index ON authorization_denials (occurred_at DESC, id);
-- +goose StatementBegin
CREATE FUNCTION reject_authorization_denial_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'authorization denials are append-only' USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER authorization_denials_append_only BEFORE UPDATE OR DELETE ON authorization_denials FOR EACH ROW EXECUTE FUNCTION reject_authorization_denial_mutation();
-- +goose Down
SELECT 1;
