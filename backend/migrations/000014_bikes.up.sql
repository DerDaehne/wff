CREATE TABLE bikes (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    chain_interval_km DOUBLE PRECISION NOT NULL DEFAULT 3000,
    chain_replaced_at_km DOUBLE PRECISION NOT NULL DEFAULT 0,
    retired_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX bikes_user_idx ON bikes (user_id);

ALTER TABLE users ADD COLUMN active_bike_id BIGINT REFERENCES bikes (id) ON DELETE SET NULL;
ALTER TABLE activities ADD COLUMN bike_id BIGINT REFERENCES bikes (id) ON DELETE SET NULL;

CREATE INDEX activities_bike_idx ON activities (bike_id);
