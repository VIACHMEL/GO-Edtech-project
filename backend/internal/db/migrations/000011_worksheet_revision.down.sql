DROP TRIGGER worksheet_revision_update ON worksheets;
DROP FUNCTION bump_worksheet_revision();
ALTER TABLE worksheets DROP COLUMN revision;
