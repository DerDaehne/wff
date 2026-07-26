-- Body weight turns climbing speed (VAM) into a power estimate:
-- W/kg ≈ VAM / (200 + 10 × grade%). That is the only way a rider without a
-- power meter gets a number they can compare against other riders and against
-- their own past (#610).
--
-- Nullable on purpose: everything else in the app works without it, and asking
-- for someone's weight before showing them anything would be a poor trade.
ALTER TABLE users ADD COLUMN weight_kg DOUBLE PRECISION;
