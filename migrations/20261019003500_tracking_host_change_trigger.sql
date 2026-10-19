-- +goose Up
-- The change log behind the service read of Tracking domain hosts (ADR 0038). A row of
-- sending_domains is a verified host while tracking_state is dns_verified or serving. Each
-- change of that is logged in the statement that makes it, from every write path (the
-- recheck job, the console, a delete). The advisory lock is held
-- to the end of the transaction, so log ids commit in the order they were taken and a
-- reader that passed an id cannot miss an entry committed below it.
-- +goose StatementBegin
CREATE FUNCTION log_tracking_host_change() RETURNS trigger AS $$
DECLARE
  old_active boolean := false;
  new_active boolean := false;
BEGIN
  IF TG_OP IN ('UPDATE', 'DELETE') THEN
    old_active := OLD.tracking_state IN ('dns_verified', 'serving');
  END IF;
  IF TG_OP IN ('INSERT', 'UPDATE') THEN
    new_active := NEW.tracking_state IN ('dns_verified', 'serving');
  END IF;

  IF old_active AND (
    TG_OP = 'DELETE' OR NOT new_active
    OR NEW.domain <> OLD.domain OR NEW.workspace_id <> OLD.workspace_id
  ) THEN
    PERFORM pg_advisory_xact_lock(7204002);
    INSERT INTO tracking_host_changes (workspace_id, domain, state, created_at)
    VALUES (OLD.workspace_id, OLD.domain, 'removed', now());
  END IF;

  IF new_active AND (
    NOT old_active
    OR NEW.domain <> OLD.domain OR NEW.workspace_id <> OLD.workspace_id
    OR NEW.tracking_state <> OLD.tracking_state
  ) THEN
    PERFORM pg_advisory_xact_lock(7204002);
    INSERT INTO tracking_host_changes (workspace_id, domain, state, created_at)
    VALUES (NEW.workspace_id, NEW.domain, NEW.tracking_state, now());
  END IF;

  RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER sending_domains_tracking_host_change
  AFTER INSERT OR DELETE OR UPDATE OF tracking_state, domain, workspace_id ON sending_domains
  FOR EACH ROW EXECUTE FUNCTION log_tracking_host_change();

-- Hosts verified before the trigger existed enter the log once.
INSERT INTO tracking_host_changes (workspace_id, domain, state, created_at)
SELECT workspace_id, domain, tracking_state, now()
FROM sending_domains
WHERE tracking_state IN ('dns_verified', 'serving')
ORDER BY id;

-- +goose Down
DROP TRIGGER sending_domains_tracking_host_change ON sending_domains;
DROP FUNCTION log_tracking_host_change();
