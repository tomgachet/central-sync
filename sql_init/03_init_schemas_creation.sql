--Create the schemas that will host the data from ODK Central
--The owner remains the superuser of the server (replace the superuser role name to fit your name)
--Do not change the schema names
--You need to connect to the database created previously

CREATE SCHEMA central_datasets AUTHORIZATION superuser;
CREATE SCHEMA central_submissions AUTHORIZATION superuser;
CREATE SCHEMA central_metadata AUTHORIZATION superuser;


-- Grant your central_user USAGE and CREATE privileges on the schemas created (replace the role name to fit your name)


GRANT ALL ON SCHEMA central_datasets TO central_user;
GRANT ALL ON SCHEMA central_submissions TO central_user;
GRANT ALL ON SCHEMA central_metadata TO central_user;