CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE users (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE activities (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    external_uid TEXT NOT NULL,
    sport TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    timezone TEXT,
    elapsed_seconds INTEGER NOT NULL,
    moving_seconds INTEGER NOT NULL,
    distance_meters DOUBLE PRECISION,
    elevation_gain_meters DOUBLE PRECISION,
    avg_power_watts DOUBLE PRECISION,
    max_power_watts DOUBLE PRECISION,
    normalized_power_watts DOUBLE PRECISION,
    avg_heart_rate DOUBLE PRECISION,
    max_heart_rate DOUBLE PRECISION,
    avg_cadence DOUBLE PRECISION,
    max_cadence DOUBLE PRECISION,
    intensity_factor DOUBLE PRECISION,
    training_stress_score DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, external_uid)
);

CREATE INDEX activities_user_started_at_idx ON activities (user_id, started_at DESC);
