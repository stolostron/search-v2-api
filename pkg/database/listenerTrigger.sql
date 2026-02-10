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

    new_data_size := OCTET_LENGTH(NEW.data::text);
    old_data_size := OCTET_LENGTH(OLD.data::text);

    -- Prepare the old and new data as JSON
    -- Truncate the data if it's too large. Notification limit is 8000 bytes,
    -- using 7000 to leave room for other fields.
    IF TG_OP = 'DELETE' THEN
        new_data_json := NULL;
        IF old_data_size < 7000 THEN
            old_data_json := OLD.data;
        ELSE
            old_data_json := NULL;
        END IF;
    ELSIF TG_OP = 'INSERT' THEN
        IF new_data_size < 7000 THEN    
            new_data_json := NEW.data;
        ELSE
            new_data_json := NULL;
        END IF;
        old_data_json := NULL;
    ELSIF TG_OP = 'UPDATE' THEN
        IF (new_data_size + old_data_size) < 7000 THEN
            new_data_json := NEW.data;
            old_data_json := OLD.data;
        ELSE IF old_data_size < 7000 THEN
            new_data_json := NULL;
            old_data_json := OLD.data;
        ELSE
            new_data_json := NULL;
            old_data_json := NULL;
        END IF;
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