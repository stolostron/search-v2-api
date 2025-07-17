-- PostgreSQL setup for LISTEN/NOTIFY functionality on search.resources table
-- This file contains the SQL commands to set up triggers for real-time notifications

-- Drop existing triggers and functions if they exist
DROP TRIGGER IF EXISTS search_resources_notify_trigger ON search.resources;
DROP FUNCTION IF EXISTS search.notify_resources_change();

-- Create the notification function
CREATE OR REPLACE FUNCTION search.notify_resources_change()
RETURNS trigger AS $$
DECLARE
    notification_payload json;
    old_data_json json;
    new_data_json json;
BEGIN
    -- Prepare the old and new data as JSON
    IF TG_OP = 'DELETE' THEN
        old_data_json := OLD.data;
        new_data_json := NULL;
    ELSIF TG_OP = 'INSERT' THEN
        old_data_json := NULL;
        new_data_json := NEW.data;
    ELSIF TG_OP = 'UPDATE' THEN
        old_data_json := OLD.data;
        new_data_json := NEW.data;
    END IF;

    -- Build the notification payload
    notification_payload := json_build_object(
        'operation', TG_OP,
        'table', TG_TABLE_NAME,
        'uid', COALESCE(NEW.uid, OLD.uid),
        'cluster', COALESCE(NEW.cluster, OLD.cluster),
        'old_data', old_data_json,
        'new_data', new_data_json,
        'timestamp', EXTRACT(EPOCH FROM NOW())::bigint
    );

    -- Send the notification
    PERFORM pg_notify('search_resources_changes', notification_payload::text);

    -- Return the appropriate record
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    ELSE
        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Create the trigger
CREATE TRIGGER search_resources_notify_trigger
    AFTER INSERT OR UPDATE OR DELETE ON search.resources
    FOR EACH ROW
    EXECUTE FUNCTION search.notify_resources_change();

-- Optional: Create trigger with conditions to filter notifications
-- This example only sends notifications for certain resource kinds
CREATE OR REPLACE FUNCTION search.notify_resources_change_filtered()
RETURNS trigger AS $$
DECLARE
    notification_payload json;
    old_data_json json;
    new_data_json json;
    resource_kind text;
    should_notify boolean := false;
BEGIN
    -- Determine if we should send a notification based on resource kind
    IF TG_OP = 'DELETE' THEN
        resource_kind := OLD.data->>'kind';
        old_data_json := OLD.data;
        new_data_json := NULL;
    ELSIF TG_OP = 'INSERT' THEN
        resource_kind := NEW.data->>'kind';
        old_data_json := NULL;
        new_data_json := NEW.data;
    ELSIF TG_OP = 'UPDATE' THEN
        resource_kind := NEW.data->>'kind';
        old_data_json := OLD.data;
        new_data_json := NEW.data;
    END IF;

    -- Filter by resource kinds (uncomment and modify as needed)
    -- You can customize this logic based on your filtering requirements
    -- Example: Only notify for Pods, Deployments, and Services
    /*
    IF resource_kind IN ('Pod', 'Deployment', 'Service', 'ConfigMap', 'Secret') THEN
        should_notify := true;
    END IF;
    */
    
    -- For now, notify for all resources (remove this line if using filtering above)
    should_notify := true;

    -- Only send notification if conditions are met
    IF should_notify THEN
        -- Build the notification payload
        notification_payload := json_build_object(
            'operation', TG_OP,
            'table', TG_TABLE_NAME,
            'uid', COALESCE(NEW.uid, OLD.uid),
            'cluster', COALESCE(NEW.cluster, OLD.cluster),
            'old_data', old_data_json,
            'new_data', new_data_json,
            'timestamp', EXTRACT(EPOCH FROM NOW())::bigint
        );

        -- Send the notification
        PERFORM pg_notify('search_resources_changes', notification_payload::text);
    END IF;

    -- Return the appropriate record
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    ELSE
        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- To use the filtered version instead, uncomment these lines:
-- DROP TRIGGER IF EXISTS search_resources_notify_trigger ON search.resources;
-- CREATE TRIGGER search_resources_notify_trigger
--     AFTER INSERT OR UPDATE OR DELETE ON search.resources
--     FOR EACH ROW
--     EXECUTE FUNCTION search.notify_resources_change_filtered();

-- Create indexes to improve notification performance if needed
-- (These are likely already present in your schema)
-- CREATE INDEX IF NOT EXISTS idx_resources_kind ON search.resources USING GIN ((data -> 'kind'));
-- CREATE INDEX IF NOT EXISTS idx_resources_namespace ON search.resources USING GIN ((data -> 'namespace'));
-- CREATE INDEX IF NOT EXISTS idx_resources_cluster ON search.resources (cluster);

-- Grant necessary permissions (adjust user as needed)
-- GRANT USAGE ON SCHEMA search TO searchuser;
-- GRANT SELECT, INSERT, UPDATE, DELETE ON search.resources TO searchuser;

-- Test the notification system (uncomment to test)
-- Listen for notifications in one session:
-- LISTEN search_resources_changes;

-- Then in another session, insert/update/delete a resource:
-- INSERT INTO search.resources (uid, cluster, data) VALUES 
--   ('test-uid', 'test-cluster', '{"kind": "Pod", "name": "test-pod", "namespace": "default"}');

-- UPDATE search.resources SET data = data || '{"updated": true}' WHERE uid = 'test-uid';
-- DELETE FROM search.resources WHERE uid = 'test-uid'; 