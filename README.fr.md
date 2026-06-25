# central-sync

`central-sync` synchronise les données d'une instance ODK Central vers un serveur PostgreSQL via l'API ODK Central et ses endpoints OData.

Il est conçu pour des synchronisations planifiées ou manuelles dans lesquelles ODK Central reste la source de vérité, tandis que PostgreSQL fournit des données structurées et interrogeables pour les workflows métier, les systèmes d'information aval, le reporting et les intégrations applicatives.

## Fonctionnalités

- Synchronise les datasets ODK Central, aussi appelés listes d'entités.
- Synchronise les soumissions de formulaires, y compris les groupes répétés.
- Crée et fait évoluer les tables PostgreSQL lorsque les schémas Central changent.
- Stocke les valeurs géométriques en GeoJSON.
- Trace les exécutions de synchronisation et les détails ligne par ligne dans `central_metadata`.
- Prend en charge la synchronisation incrémentale des formulaires avec les modes `append_only` et `upsert`.
- Peut restreindre la synchronisation des formulaires aux soumissions approuvées.
- Peut approuver les soumissions dans Central après une synchronisation réussie en base de données.
- S'exécute sous forme d'un binaire Go unique avec des fichiers de configuration locaux.

## Prérequis

- Un compte ODK Central ayant accès aux projets, datasets et formulaires à synchroniser.
- Une base PostgreSQL pour chaque projet ODK Central configuré.
- Un rôle PostgreSQL ayant accès aux schémas de synchronisation.
- Les fichiers suivants à côté du binaire :
  - `.env`
  - `central_config.yaml`

Pour le développement, Go est requis. Le projet cible actuellement Go `1.26.x` en CI.

## Installation

Téléchargez un binaire depuis la page des releases, puis placez-le dans un répertoire contenant `.env` et `central_config.yaml`.

Depuis les sources :

```sh
go test ./...
go build -o central-sync
```

## Fichier d'environnement

Créez `.env` à côté du binaire :

```env
ODK_CENTRAL_URL=https://central.example.org
ODK_CENTRAL_USER_EMAIL=user@example.org
ODK_CENTRAL_USER_PASSWORD=your_password

PG_HOST=localhost
PG_PORT=5432
PG_USER=central_user
PG_PASSWORD=pg_central_user_password
PG_SSLMODE=disable
```

Variables requises :

| Variable | Description |
| --- | --- |
| `ODK_CENTRAL_URL` | URL de base de l'instance ODK Central. |
| `ODK_CENTRAL_USER_EMAIL` | Email de connexion Central. |
| `ODK_CENTRAL_USER_PASSWORD` | Mot de passe Central. |
| `PG_HOST` | Hôte PostgreSQL. |
| `PG_PORT` | Port PostgreSQL. |
| `PG_USER` | Rôle PostgreSQL utilisé par `central-sync`. |
| `PG_SSLMODE` | Mode SSL PostgreSQL, par exemple `disable` ou `require`. |

`PG_PASSWORD` est optionnel au moment du parsing, mais la plupart des configurations PostgreSQL l'exigent.

Ne commitez pas `.env`. Il contient des identifiants, et des tokens Central peuvent aussi être cachés localement par le programme.

## Configuration des projets

Créez `central_config.yaml` à côté du binaire :

```yaml
projects:
  - project_id: 1
    project_name: "Example project"
    database_name: "central_project_1"
    datasets:
      - name: "species"
        table_name: "species"
        sync: true
      - name: "sites"
        table_name: "sites"
        sync: false
    forms:
      - xml_form_id: "site_visit"
        table_name: "site_visit"
        sync: true
        sync_mode: "upsert"
        approved_only: true
        approve_after_sync: false

  - project_id: 2
    project_name: "Another project"
    database_name: "central_project_2"
    datasets: []
    forms: []
```

### Champs projet

| Champ | Requis | Description |
| --- | --- | --- |
| `project_id` | Oui | ID numérique du projet ODK Central. Doit être supérieur à `0`. |
| `project_name` | Non | Nom informatif uniquement. |
| `database_name` | Oui | Base PostgreSQL cible pour ce projet. |
| `datasets` | Non | Mappings des datasets à synchroniser. |
| `forms` | Non | Mappings des formulaires à synchroniser. |

### Champs dataset

| Champ | Requis | Description |
| --- | --- | --- |
| `name` | Oui | Nom du dataset/de la liste d'entités dans ODK Central. |
| `table_name` | Oui | Nom de la table cible dans `central_datasets`. |
| `sync` | Oui | Seules les entrées avec `sync: true` sont synchronisées. |

### Champs formulaire

| Champ | Requis | Description |
| --- | --- | --- |
| `xml_form_id` | Oui | XML form ID ODK Central. |
| `table_name` | Oui | Nom de la table racine cible dans `central_submissions`. Les tables de repeat sont dérivées de ce nom. |
| `sync` | Oui | Seules les entrées avec `sync: true` sont synchronisées. |
| `sync_mode` | Non | `append_only` par défaut. Peut valoir `append_only` ou `upsert`. |
| `approved_only` | Non | Quand `true`, seules les soumissions approuvées sont récupérées. |
| `approve_after_sync` | Non | Quand `true`, les soumissions synchronisées avec succès sont approuvées dans Central après le commit PostgreSQL. |

Les noms de tables doivent être uniques au sein d'un même projet, entre les mappings datasets et formulaires.

## Configuration PostgreSQL

Chaque projet configuré pointe vers une base PostgreSQL. Cette base doit déjà exister et contenir les schémas et tables de métadonnées requis.

Exécutez les scripts d'initialisation avec un rôle PostgreSQL privilégié, après avoir adapté les noms de base et de rôle :

```sh
psql -d your_database -f sql_init/01_init_structure.sql
psql -d your_database -f sql_init/02_init_role_and_privileges.sql
```

Le script de structure crée :

- `central_datasets`
- `central_submissions`
- `central_metadata`
- `central_metadata.sync_runs`
- `central_metadata.sync_runs_detail`
- les vues de métadonnées utilisées pour la synchronisation incrémentale et le suivi des reprises

Le script de privilèges est un template. Remplacez `your_central_user`, `your_central_user_password` et `your_database` avant de l'exécuter.

Une configuration de rôle typique est :

```sql
CREATE ROLE central_user WITH
    NOSUPERUSER
    NOCREATEDB
    NOCREATEROLE
    NOINHERIT
    LOGIN
    NOREPLICATION
    NOBYPASSRLS
    CONNECTION LIMIT -1
    PASSWORD 'pg_central_user_password';

GRANT CONNECT ON DATABASE your_database TO central_user;
GRANT ALL ON SCHEMA central_datasets TO central_user;
GRANT ALL ON SCHEMA central_submissions TO central_user;
GRANT ALL ON SCHEMA central_metadata TO central_user;
```

Utilisez un rôle PostgreSQL dédié plutôt qu'un superuser pour les synchronisations courantes.

## Exécution

Lancez le binaire depuis le répertoire contenant `.env` et `central_config.yaml` :

```sh
./central-sync
```

Afficher la version :

```sh
./central-sync version
```

Le programme s'exécute dans cet ordre :

1. Charge `.env`.
2. Charge et valide `central_config.yaml`.
3. S'authentifie auprès d'ODK Central.
4. Synchronise les datasets configurés.
5. Synchronise les formulaires configurés.
6. Écrit les logs dans stdout et `central-sync.log`.

## Endpoints Central utilisés

`central-sync` utilise `ODK_CENTRAL_URL` comme URL de base. L'utilisateur Central configuré doit disposer des permissions nécessaires pour chaque projet, dataset, formulaire et action de soumission ci-dessous.

| Méthode | Endpoint | Utilisation |
| --- | --- | --- |
| `POST` | `/v1/sessions` | Crée ou rafraîchit le token de session Central. |
| `GET` | `/v1/projects/{projectId}` | Vérifie qu'un projet configuré existe. |
| `GET` | `/v1/projects/{projectId}/forms` | Liste les formulaires du projet et valide les valeurs `xml_form_id` configurées. |
| `GET` | `/v1/projects/{projectId}/forms/{xmlFormId}.svc` | Lit le document de service OData du formulaire et découvre les tables racine et repeat. |
| `GET` | `/v1/projects/{projectId}/forms/{xmlFormId}.svc/$metadata` | Lit les métadonnées OData XML pour les schémas des tables de formulaire. |
| `GET` | `/v1/projects/{projectId}/forms/{xmlFormId}.svc/{odataTableUrl}` | Récupère les lignes de soumission depuis les tables OData, y compris `Submissions` et les tables repeat. Utilise `$top=1000`, `$count=true`, un `$filter` optionnel, et suit `@odata.nextLink`. |
| `PATCH` | `/v1/projects/{projectId}/forms/{xmlFormId}/submissions/{instanceId}` | Définit `reviewState` à `approved` quand `approve_after_sync` est activé. |
| `POST` | `/v1/projects/{projectId}/forms/{xmlFormId}/submissions/{instanceId}/comments` | Ajoute un commentaire de synchronisation après approbation. |
| `GET` | `/v1/projects/{projectId}/datasets/{datasetName}` | Lit les métadonnées et propriétés du dataset. |
| `GET` | `/v1/projects/{projectId}/datasets/{datasetName}.svc/Entities` | Récupère les entités du dataset via OData. Utilise `$top=1000`, `$count=true`, un `$filter` optionnel, et suit `@odata.nextLink`. |
| `GET` | `/v1/projects/{projectId}/datasets/{datasetName}/entities.geojson` | Récupère les géométries du dataset en GeoJSON lorsque des valeurs géométriques sont présentes. |

Pour tester manuellement l'API, voir le dépôt [`central-api-bruno`](https://github.com/tomgachet/central-api-bruno), qui fournit une collection Bruno pour tester les endpoints de l'API Central.

## Comportement de synchronisation

### Filtres

`central-sync` construit les expressions OData `$filter` à partir de la dernière synchronisation réussie exposée par les vues SQL dans `central_metadata`, en particulier `last_successful_submissions_sync` et `last_successful_datasets_sync`. Ces filtres limitent chaque exécution aux enregistrements modifiés après le dernier curseur réussi pour le même projet et le même dataset ou formulaire.

Pour les datasets, les entités actives sont récupérées lorsque `__system/createdAt` ou `__system/updatedAt` est supérieur au curseur de synchronisation réussie précédent. Les entités supprimées sont récupérées séparément avec `__system/deletedAt ne null`; après la première synchronisation réussie des entités supprimées, ce filtre est restreint à `__system/deletedAt gt <last_deleted_at>`.

Pour les soumissions de formulaires, le filtre dépend de `sync_mode` :

| Mode | Comportement du filtre |
| --- | --- |
| `append_only` | Récupère les soumissions dont `submissionDate` est supérieur au curseur de soumission réussie précédent. |
| `upsert` | Récupère les soumissions dont `submissionDate` ou `updatedAt` est supérieur au curseur réussi précédent. |

Quand `approved_only: true`, le filtre du formulaire exige aussi `reviewState eq 'approved'`. Pour les tables repeat, les mêmes champs système de soumission sont adressés via `$root/Submissions/__system/...`, ce qui permet aux lignes repeat de suivre le même filtre que la soumission racine.

Les soumissions de formulaire en échec sont aussi rejouées en dehors du filtre incrémental normal. `central-sync` lit `central_metadata.last_failed_submissions`, récupère de nouveau ces UUID de soumission depuis Central, puis les fusionne avec les lignes retournées par le filtre OData incrémental classique afin de les retenter dans la même exécution.

Lors d'une première exécution, aucun curseur de synchronisation réussie n'existe encore ; le filtre incrémental de date est donc omis et Central retourne les lignes correspondant au dataset ou formulaire configuré.

### Datasets

Les datasets sont synchronisés dans le schéma `central_datasets`. Le programme crée ou met à jour les tables cibles à partir du schéma d'entités Central et trace les lignes traitées dans `central_metadata.sync_runs_detail`.

### Formulaires

Les soumissions de formulaires sont synchronisées dans le schéma `central_submissions`.

La table racine `Submissions` utilise le `table_name` configuré. Les tables repeat sont dérivées du nom de la table racine et du chemin repeat OData.

Modes de synchronisation de formulaire pris en charge :

| Mode | Comportement |
| --- | --- |
| `append_only` | Récupère les nouvelles soumissions à partir de la date de soumission Central et insère les lignes qui n'ont pas déjà été synchronisées. |
| `upsert` | Récupère les soumissions à partir de la date de soumission Central ou de la date de mise à jour, puis met à jour les lignes existantes si nécessaire. |

Si `sync_mode` est vide, `append_only` est utilisé.

### Soumissions approuvées uniquement

Définissez `approved_only: true` sur un formulaire pour ne récupérer que les soumissions dont l'état de revue Central est `approved`.

Cette option filtre uniquement ce qui est récupéré depuis Central. Elle n'approuve rien par elle-même.

### Approbation après synchronisation

Définissez `approve_after_sync: true` sur un formulaire pour approuver chaque soumission racine dans ODK Central après que ses lignes ont été validées avec succès dans PostgreSQL.

La séquence est volontairement ordonnée ainsi :

1. Insérer ou mettre à jour les lignes de soumission dans PostgreSQL au sein d'une transaction.
2. Committer la transaction PostgreSQL.
3. Approuver la soumission dans ODK Central.
4. Ajouter un commentaire de synchronisation dans ODK Central.

Cela garantit qu'une soumission n'est pas approuvée dans Central avant d'avoir été stockée localement.

Ce processus n'est pas atomique entre PostgreSQL et ODK Central. Si le commit en base réussit mais que l'approbation Central échoue, les données restent synchronisées dans PostgreSQL et l'exécution enregistre un détail en échec avec `sync_action = 'approve_after_sync_failed'`. Si l'approbation réussit mais que le commentaire de synchronisation échoue, l'exécution enregistre `sync_action = 'approve_comment_failed'`.

Traitez ces cas comme des échecs partiels récupérables et inspectez `central_metadata.sync_runs` et `central_metadata.sync_runs_detail` pour identifier les soumissions concernées.

## Suivi et journalisation

`central-sync` écrit les logs dans stdout et dans `central-sync.log`.

Chaque synchronisation de dataset ou de formulaire crée des enregistrements dans `central_metadata.sync_runs`. Les détails ligne par ligne sont écrits dans `central_metadata.sync_runs_detail` avec les compteurs de lignes récupérées, insérées, mises à jour, ignorées, supprimées et en échec.

Objets de métadonnées utiles :

| Objet | Utilisation |
| --- | --- |
| `central_metadata.sync_runs` | Une ligne par exécution de synchronisation de haut niveau. |
| `central_metadata.sync_runs_detail` | Événements détaillés au niveau ligne ou soumission. |
| `central_metadata.last_successful_submissions_sync` | Curseur incrémental pour les soumissions de formulaires. |
| `central_metadata.last_successful_datasets_sync` | Curseur incrémental pour les datasets. |
| `central_metadata.last_failed_submissions` | Derniers événements de soumission en échec utilisés pour le suivi des reprises. |

## Développement

Exécuter les tests localement :

```sh
go test ./...
```

Le dépôt inclut aussi un workflow GitHub Actions qui exécute la même commande sur `push` et `pull_request`.

Avant d'ouvrir une pull request, gardez les fichiers locaux sans rapport hors du commit. Les notes locales comme les documents d'audit ou les notes de prochaines étapes ne doivent être ajoutées au commit que si elles sont destinées à devenir de la documentation du projet.

## Contribution

Les contributions sont bienvenues. Les rapports de bugs, améliorations de documentation, tests et changements de code qui rendent la synchronisation plus sûre ou plus simple à opérer sont utiles.

Ouvrez une issue ou une pull request avec une description claire du problème, du changement proposé et de tout contexte utile sur votre configuration ODK Central ou PostgreSQL.
