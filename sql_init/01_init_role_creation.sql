-- Create a role with login option that connects to PostgreSQL via central-sync (replace the name and password as you wish)

CREATE ROLE central_user WITH 
	NOSUPERUSER
	NOCREATEDB
	NOCREATEROLE
	NOINHERIT
	LOGIN
	NOREPLICATION
	NOBYPASSRLS
	CONNECTION LIMIT -1
    PASSWORD 'central_user_password';