CREATE TABLE IF NOT EXISTS teams (
  team_name text PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS users (
  user_id   text PRIMARY KEY,
  username  text NOT NULL,
  team_name text NOT NULL REFERENCES teams(team_name) ON DELETE RESTRICT,
  is_active boolean NOT NULL DEFAULT true
);

CREATE TABLE IF NOT EXISTS pull_requests (
  pull_request_id   text PRIMARY KEY,
  pull_request_name text NOT NULL,
  author_id         text NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
  status            text NOT NULL DEFAULT 'OPEN'
    CHECK (status IN ('OPEN','MERGED')),
  created_at        timestamptz NOT NULL DEFAULT now(),
  merged_at         timestamptz
);

CREATE TABLE IF NOT EXISTS pr_reviewers (
  pr_id       text NOT NULL REFERENCES pull_requests(pull_request_id) ON DELETE CASCADE,
  slot        smallint NOT NULL CHECK (slot IN (1,2)),
  reviewer_id text NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
  assigned_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (pr_id, slot),
  UNIQUE (pr_id, reviewer_id)
);

CREATE OR REPLACE FUNCTION prevent_reviewers_change_on_merged()
RETURNS trigger AS $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM pull_requests pr
    WHERE pr.pull_request_id = COALESCE(NEW.pr_id, OLD.pr_id)
      AND pr.status = 'MERGED'
  ) THEN
    RAISE EXCEPTION 'Cannot modify reviewers for merged PR'
      USING ERRCODE = '55000';
  END IF;

  RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_no_reviewers_change_after_merge ON pr_reviewers;

CREATE TRIGGER trg_no_reviewers_change_after_merge
BEFORE INSERT OR UPDATE OR DELETE ON pr_reviewers
FOR EACH ROW
EXECUTE FUNCTION prevent_reviewers_change_on_merged();
