ALTER TABLE worksheets ADD COLUMN revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0);

CREATE FUNCTION bump_worksheet_revision() RETURNS trigger AS $$
BEGIN
    NEW.revision := OLD.revision + 1;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER worksheet_revision_update BEFORE UPDATE ON worksheets
FOR EACH ROW EXECUTE FUNCTION bump_worksheet_revision();
