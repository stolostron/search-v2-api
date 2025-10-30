-- Copyright Contributors to the Open Cluster Management project

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
    new_data_json json;
BEGIN
    -- Prepare the old and new data as JSON
    IF TG_OP = 'DELETE' THEN
        new_data_json := NULL;
        old_data_json := OLD.data;
    ELSIF TG_OP = 'INSERT' THEN
        new_data_json := NEW.data;
        old_data_json := NULL;
    ELSIF TG_OP = 'UPDATE' THEN
        new_data_json := NEW.data;
        old_data_json := NULL; -- Currently not used, keeping for future use.
    END IF;

    -- Build the notification payload
    notification_payload := json_build_object(
        'operation', TG_OP,
        'uid', COALESCE(NEW.uid, OLD.uid),
        'cluster', COALESCE(NEW.cluster, OLD.cluster),
        'new_data', new_data_json,
        'old_data', old_data_json,
        'timestamp', EXTRACT(EPOCH FROM NOW())::bigint
    );

    -- Send the notification
    NOTIFY search_resources_changes, notification_payload::text;

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