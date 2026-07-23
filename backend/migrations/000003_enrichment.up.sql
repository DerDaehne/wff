CREATE TABLE enrichment (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    activity_id BIGINT NOT NULL REFERENCES activities (id) ON DELETE CASCADE,
    hour_bucket TIMESTAMPTZ NOT NULL,
    lat DOUBLE PRECISION NOT NULL,
    lon DOUBLE PRECISION NOT NULL,
    temperature_celsius DOUBLE PRECISION,
    wind_speed_mps DOUBLE PRECISION,
    wind_direction_deg DOUBLE PRECISION,
    precipitation_mm DOUBLE PRECISION,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (activity_id, hour_bucket)
);
