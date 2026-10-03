ALTER TABLE steps ADD CONSTRAINT steps_id_module_version_unique UNIQUE(id,module_version_id);
ALTER TABLE learning_attempts ADD CONSTRAINT learning_attempts_step_version_fk
 FOREIGN KEY(step_id,module_version_id) REFERENCES steps(id,module_version_id);

CREATE FUNCTION reject_published_content_change() RETURNS trigger AS $$
BEGIN
 RAISE EXCEPTION 'published content is immutable; create a new version';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER case_versions_immutable BEFORE UPDATE OR DELETE ON case_versions
 FOR EACH ROW EXECUTE FUNCTION reject_published_content_change();
CREATE TRIGGER module_versions_immutable BEFORE UPDATE OR DELETE ON module_versions
 FOR EACH ROW EXECUTE FUNCTION reject_published_content_change();
CREATE TRIGGER steps_immutable BEFORE UPDATE OR DELETE ON steps
 FOR EACH ROW EXECUTE FUNCTION reject_published_content_change();
