-- +goose Up

ALTER TABLE approval_requests
    ADD COLUMN superseded_by UUID REFERENCES pipeline_runs(id);

-- +goose Down

ALTER TABLE approval_requests DROP COLUMN superseded_by;
