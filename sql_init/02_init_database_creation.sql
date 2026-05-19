-- Create the databases linked to the projects to be synchronized (replace the database name as you wish)
-- The owner remains the superuser of the server (replace the superuser role name example to fit yours)
-- Create as many databases as there are projects to synchronize

CREATE DATABASE target_database_name_for_project_1
    WITH
    OWNER = postgres_superuser
    ENCODING = 'UTF8'
    TABLESPACE = pg_default
    CONNECTION LIMIT = -1
    IS_TEMPLATE = False;

-- Grant your central_user the privilege to connect on the databases created (replace the database and the role name example to fit yours)

GRANT CONNECT ON DATABASE target_database_name_for_project_1 TO central_user;