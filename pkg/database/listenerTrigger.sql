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
    new_data_size integer;
    old_data_size integer;
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

    new_data_size := OCTET_LENGTH(new_data_json::text);
    old_data_size := OCTET_LENGTH(old_data_json::text);

    -- The total size of the payload must be less than 8000 bytes. Using 7000 for extra safety.
    IF old_data_size + new_data_size > 7000 THEN
        -- Remove new_data_json first, the receiver can query for the full current data.
        new_data_json := NULL;

        IF old_data_size > 7000 THEN
            -- LIMITATION: We can't query for OLD.data later, will need to save in a separate table.
            old_data_json := NULL;
        END IF;
        RAISE WARNING 'Payload truncated.'
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

    -- Check payload size and send the notification.
    IF OCTET_LENGTH(notification_payload::text) < 7500 THEN
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