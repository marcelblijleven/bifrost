-- +goose Up
ALTER TABLE pipeline_step_results ADD COLUMN external_run_id BIGINT;
CREATE INDEX idx_step_results_external_run_id ON pipeline_step_results (external_run_id) WHERE external_run_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_step_results_external_run_id;
ALTER TABLE pipeline_step_results DROP COLUMN external_run_id;
