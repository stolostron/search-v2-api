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
    old_data_json json;
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
        old_data_json := OLD.data;
    END IF;

    -- Build the notification payload
    notification_payload := json_build_object(
        'operation', TG_OP,
        'uid', COALESCE(NEW.uid, OLD.uid),
        'cluster', COALESCE(NEW.cluster, OLD.cluster),
        'newData', new_data_json,
        'oldData', old_data_json,
        'timestamp', NOW()
    );

    payload_size := OCTET_LENGTH(notification_payload::text);

    IF payload_size > 8000 THEN
        RAISE WARNING 'Payload size is too large: % bytes', payload_size;
        -- TODO: Send a different paylod with only the uid and timestamp.
    ELSE
        -- Send the notification
        PERFORM pg_notify('search_resources_notify', notification_payload::text);
    END IF;


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