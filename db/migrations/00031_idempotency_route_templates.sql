-- +goose Up
-- Idempotency is keyed by stable OpenAPI route templates. Route parameters
-- are represented with braces, so persist that canonical form explicitly.
ALTER TABLE idempotency_records
    DROP CONSTRAINT idempotency_records_route_canonical;
ALTER TABLE idempotency_records
    ADD CONSTRAINT idempotency_records_route_canonical
    CHECK (route ~ '^/[A-Za-z0-9._~!$&''()*+,;=:@/%{}-]*$');

-- +goose Down
ALTER TABLE idempotency_records
    DROP CONSTRAINT idempotency_records_route_canonical;
ALTER TABLE idempotency_records
    ADD CONSTRAINT idempotency_records_route_canonical
    CHECK (route ~ '^/[A-Za-z0-9._~!$&''()*+,;=:@/%-]*$');
