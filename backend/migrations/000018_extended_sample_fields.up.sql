ALTER TABLE samples
    ADD COLUMN grade_percent DOUBLE PRECISION,
    ADD COLUMN calories_kcal INTEGER,
    ADD COLUMN left_right_balance_percent DOUBLE PRECISION,
    ADD COLUMN left_right_balance_right_leg BOOLEAN,
    ADD COLUMN left_torque_effectiveness_percent DOUBLE PRECISION,
    ADD COLUMN right_torque_effectiveness_percent DOUBLE PRECISION,
    ADD COLUMN left_pedal_smoothness_percent DOUBLE PRECISION,
    ADD COLUMN right_pedal_smoothness_percent DOUBLE PRECISION,
    ADD COLUMN combined_pedal_smoothness_percent DOUBLE PRECISION,
    ADD COLUMN gps_accuracy_meters INTEGER,
    ADD COLUMN resistance INTEGER;
