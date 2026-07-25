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

Téléchargez l'archive `.tar.gz` correspondant à votre plateforme depuis la page des releases, puis extrayez le binaire `central-sync` dans un répertoire contenant `.env` et `central_config.yaml`.

```sh
tar -xzf central-sync-linux-amd64.tar.gz
```

Le nom du binaire reste `central-sync` d'une version à l'autre, ce qui permet de le mettre à jour sans modifier un cron ou un script d'exploitation.

Si vous téléchargez directement le binaire brut Linux depuis GitHub, rendez-le exécutable puis renommez-le :

```sh
chmod +x central-sync-linux-amd64
mv central-sync-linux-amd64 central-sync
```

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
attachment_storage:
  backend: "local"
  local_directory: "/var/lib/central-sync/attachments"

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
        sync_attachments: true

  - project_id: 2
    project_name: "Another project"
    database_name: "central_project_2"
    datasets: []
    forms: []
```

### Champs du stockage des pièces jointes

| Champ | Requis | Description |
| --- | --- | --- |
| `backend` | Quand la synchronisation des pièces jointes est activée | Backend de stockage des pièces jointes. Seul `local` est actuellement pris en charge. |
| `local_directory` | Quand la synchronisation utilise `backend: local` | Répertoire racine dans lequel les pièces jointes des soumissions seront stockées. Les chemins relatifs sont résolus depuis le répertoire de travail du processus. |

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
| `sync_attachments` | Non | Quand `true`, les pièces jointes des soumissions sont synchronisées avec le stockage `attachment_storage` configuré. La valeur par défaut est `false`. |

Les noms de tables doivent être uniques au sein d'un même projet, entre les mappings datasets et formulaires.

### Synchronisation des pièces jointes des soumissions

Quand `sync_attachments: true`, les pièces jointes sont synchronisées après le commit des lignes de la soumission dans PostgreSQL et avant l'application de `approve_after_sync`. Les fichiers signalés comme absents par Central sont comptabilisés, mais ne sont pas téléchargés.

Si une pièce jointe déjà synchronisée est ensuite signalée avec `exists: false`, son fichier local est conservé afin d'éviter toute perte automatique de données. Ses métadonnées passent à `central_exists: false` avec une date `missing_at`, et un avertissement est journalisé. Si le fichier redevient disponible, une synchronisation réussie efface `missing_at` et rétablit `central_exists: true`.

Le backend local écrit les fichiers de manière atomique sous `local_directory` avec cette arborescence :

```text
project-{project_id}/form-{xml_form_id}/submission-{submission_uuid}/{filename}
```

Les composants des chemins sont échappés avant utilisation. Les répertoires utilisent le mode `0750` et les fichiers le mode `0640` ; l'utilisateur système qui exécute `central-sync` doit pouvoir créer et écrire sous `local_directory`. Incluez ce répertoire dans les sauvegardes avec la base du projet.

`central-sync` calcule un checksum SHA-256 pendant le téléchargement. Un fichier existant dont le contenu est identique reste inchangé. Un contenu différent remplace le fichier de manière atomique et apparaît avec l'action `replaced` dans les logs. Le checksum est conservé dans les métadonnées de la pièce jointe.

Les téléchargements de pièces jointes utilisent un timeout HTTP dédié de 10 minutes. Les autres requêtes vers l'API Central conservent leur timeout de 30 secondes.

Les métadonnées des fichiers stockés sont mises à jour par UPSERT dans `central_metadata.submission_attachments`. Une erreur de téléchargement, de stockage ou de métadonnées marque la soumission en échec, empêche son approbation automatique et la rend éligible au mécanisme existant de reprise des soumissions en échec.

Chaque ligne de formulaire dans `central_metadata.sync_runs` enregistre les totaux `attachments_expected`, `attachments_present`, `attachments_stored` et `attachments_failed` pour le suivi opérationnel.

## Configuration PostgreSQL

Chaque projet configuré pointe vers une base PostgreSQL. Cette base doit déjà exister et contenir les schémas et tables de métadonnées requis.

Exécutez les scripts d'initialisation avec un rôle PostgreSQL privilégié, après avoir adapté les noms de base et de rôle :

```sh
psql -d your_database -f sql_init/01_init_structure.sql
psql -d your_database -f sql_init/02_init_role_and_privileges.sql
```

Après avoir récupéré une nouvelle version de `central-sync`, consultez `sql_migrations/` pour repérer les fichiers postérieurs à la version précédemment installée. Exécutez les migrations requises dans l'ordre des versions sur chaque base projet avant le premier lancement du nouveau binaire.

Pour une mise à jour de 0.2.x vers 0.3.0, exécutez :

```sh
psql -d your_database -f sql_migrations/v0.3.0_add_sync_id.sql
```

Les anciennes lignes de métadonnées conservent un `sync_id` NULL, car leur lancement d'origine ne peut pas être reconstitué de manière fiable.

La migration valide également la clé étrangère de `sync_runs_detail.run_id` vers `sync_runs.run_id`. Elle s'arrête sans valider la transaction si des détails historiques orphelins existent ; corrigez ces lignes avant de relancer la migration.

Pour une mise à jour de 0.3.x vers 0.4.0, remplacez `your_central_user` dans la migration par le rôle PostgreSQL utilisé par `central-sync`, puis exécutez :

```sh
psql -d your_database -f sql_migrations/v0.4.0_add_submission_attachments.sql
```

Le script de structure crée :

- `central_datasets`
- `central_submissions`
- `central_metadata`
- `central_metadata.sync_runs`
- `central_metadata.sync_runs_detail`
- `central_metadata.submission_attachments`
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
cd /chemin/du/repertoire/central-sync
./central-sync
```

Afficher la version :

```sh
./central-sync version
```

Le programme s'exécute dans cet ordre :

1. Charge `.env`.
2. Charge et valide `central_config.yaml`.
3. Acquiert un verrou PostgreSQL advisory pour chaque base projet configurée qui contient des mappings dataset ou formulaire actifs.
4. S'authentifie auprès d'ODK Central.
5. Synchronise les datasets configurés.
6. Synchronise les formulaires configurés.
7. Écrit les logs dans stdout et `central-sync.log`.

### Exécutions concurrentes

`central-sync` échoue immédiatement lorsqu'un autre processus synchronise déjà la même base projet. Le verrou est un verrou PostgreSQL advisory conservé sur une connexion dédiée pendant toute la durée du processus ; il fonctionne donc entre processus distincts et se libère automatiquement quand le processus se termine ou quand la connexion est perdue.

La portée du verrou est une base PostgreSQL projet. Deux projets configurés sur des bases PostgreSQL différentes peuvent donc toujours se synchroniser en parallèle, mais deux exécutions visant la même base ne le peuvent pas.

Si une seconde exécution démarre pendant que le verrou est détenu, elle journalise une erreur `sync lock error` et s'arrête avant de lire ou d'écrire des données Central. Quand l'information est disponible, l'erreur inclut la dernière ligne `central_metadata.sync_runs` encore marquée `running`, avec son `run_id`, son `project_id`, le type d'objet, le nom de l'objet et l'heure de démarrage. Les exécutions terminées, échouées ou en succès partiel ne bloquent pas les exécutions suivantes.

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

Chaque lancement du processus génère un UUID nommé `sync_id`. La même valeur est stockée sur toutes les exécutions créées pendant ce lancement, y compris lorsque plusieurs bases de projets sont traitées. Les logs de démarrage, de fin, de cycle de vie des datasets/formulaires et d'erreur l'incluent afin de corréler l'exécution complète sans le répéter dans chaque message intermédiaire.

Chaque synchronisation de dataset ou de formulaire reçoit également son propre `run_id`, généré par la base, et crée un enregistrement dans `central_metadata.sync_runs`. Un `run_id` identifie la synchronisation d'un dataset ou d'un formulaire, tandis qu'un `sync_id` identifie le lancement complet de `central-sync`. Les détails ligne par ligne sont écrits dans `central_metadata.sync_runs_detail` et reliés à leur lancement par `run_id`.

Par exemple, toutes les métadonnées d'un lancement peuvent être consultées avec :

```sql
SELECT *
FROM central_metadata.sync_runs
WHERE sync_id = '00000000-0000-4000-8000-000000000000';
```

Pour inclure les détails ligne par ligne :

```sql
SELECT r.sync_id, d.*
FROM central_metadata.sync_runs AS r
JOIN central_metadata.sync_runs_detail AS d USING (run_id)
WHERE r.sync_id = '00000000-0000-4000-8000-000000000000';
```

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

## Limites connues

Les limites suivantes sont connues et seront intégrées ou améliorées dans des versions futures :

- Les index PostgreSQL restent minimaux dans cette première version publique. Des index supplémentaires sont prévus pour les recherches dans les métadonnées, les curseurs de synchronisation incrémentale, les reprises de soumissions en échec et les tables métier synchronisées.
- Les pièces jointes aux soumissions, par exemple les photos, sons ou autres fichiers média, ne sont pas encore synchronisées.
- La gestion des fichiers de logs reste minimale. `central-sync` écrit dans stdout et ajoute les messages à `central-sync.log`, mais ne gère pas encore la rotation, la rétention, la taille maximale ou un chemin de log configurable.

## Licence

`central-sync` est distribué sous licence Apache 2.0. Voir [LICENSE](LICENSE).

## Contribution

Les contributions sont bienvenues. Les rapports de bugs, améliorations de documentation, tests et changements de code qui rendent la synchronisation plus sûre ou plus simple à opérer sont utiles.

Ouvrez une issue ou une pull request avec une description claire du problème, du changement proposé et de tout contexte utile sur votre configuration ODK Central ou PostgreSQL.
