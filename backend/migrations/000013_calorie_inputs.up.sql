-- Energy expenditure from heart rate (Keytel et al. 2005) needs weight, age
-- and sex on top of the pulse. Weight already exists (migration 000010); these
-- two are what "how much did I burn?" costs (#625) — a question hobby riders
-- ask before any training metric.
--
-- Birth year rather than age: an age would quietly go stale, and a ride from
-- three years ago should be computed with the age of that season, not today's.
--
-- sex holds 'male' or 'female' because the source publishes exactly two
-- coefficient sets. It stays nullable, and nothing else in the app reads it:
-- without it there is no calorie figure, which is the honest outcome rather
-- than picking one set and hoping.
ALTER TABLE users ADD COLUMN birth_year INT;
ALTER TABLE users ADD COLUMN sex TEXT;
