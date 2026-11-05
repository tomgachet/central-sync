# Installation sur Debian 13

Cette installation utilise un binaire partagé et une instance systemd par projet
ODK Central. Chaque instance possède son `.env`, sa configuration YAML, son cache
de session, ses pièces jointes et ses logs. Elle utilise le compte système
`central-sync`, sans accès superuser PostgreSQL. Les instances partagent ce compte
Unix : la séparation par projet est une séparation d'exploitation, pas une
isolation de sécurité entre projets.

Les commandes ci-dessous sont exécutées en root sur le VPS depuis la racine du
dépôt. Elles installent les fichiers ; aucune connexion au VPS n'est effectuée
automatiquement. Go et le dépôt ne sont pas nécessaires pour les exécutions :
seuls le binaire installé et les configurations sont utilisés.

## Installation avec le CLI

L'installeur remplace les étapes manuelles 1 et 2 ci-dessous. Depuis le dépôt,
préparez le binaire de la release choisie (ou construisez cette branche) puis
installez les dépendances en root :

```sh
apt-get update
apt-get install -y ca-certificates logrotate
bash deployment/install.sh --project projet-1 --binary ./central-sync --dry-run
bash deployment/install.sh --project projet-1 --binary ./central-sync
```

Pour fournir votre configuration initiale plutôt que les exemples :

```sh
bash deployment/install.sh --project projet-1 --binary ./central-sync \
  --env-file /chemin/projet.env --config-file /chemin/projet.yaml
```

L'installeur crée le compte système, installe le binaire par remplacement
atomique, les unités et logrotate, puis prépare les répertoires et les liens.
Les configurations existantes ne sont jamais remplacées, même si des fichiers
d'entrée sont fournis. Les logs, pièces jointes et caches sont conservés.
`--dry-run` ne modifie rien et ne vérifie pas les prérequis système de Debian.
L'installation réelle exige Debian 13 avec systemd actif et les droits root.

Pour ajouter un projet en réutilisant le binaire installé :

```sh
bash deployment/install.sh --project projet-2
```

Le script ne lance aucune synchronisation et n'active aucun timer. Il conserve
l'état des timers existants. Si `--binary` est fourni, tous les timers de projet
doivent être arrêtés et les services terminés avant de remplacer le binaire.
La base PostgreSQL et ses migrations restent à préparer selon le README.
Éditez la configuration créée puis continuez à l'étape 3 pour tester et planifier.
`bash deployment/install.sh --help` affiche les options disponibles.

Pour vérifier l'installeur en développement :

```sh
bash -n deployment/install.sh
python3 -m unittest discover -s deployment/tests -v
```

Les tests simulent les commandes système et redirigent les chemins d'installation
vers un répertoire temporaire ; ils ne réalisent pas une installation sur Debian.

## 1. Installer manuellement le binaire et les fichiers système

Récupérez une release pour l'architecture du VPS (`dpkg --print-architecture`),
vérifiez son SHA-256 avec le fichier `SHA256SUMS` fourni et extrayez le binaire
`central-sync` dans le répertoire courant. Pour tester cette branche avant une
release, construisez le binaire depuis ses sources avec `go build -o central-sync`.

```sh
apt-get update
apt-get install -y ca-certificates logrotate
adduser --system --group --no-create-home central-sync
install -o root -g root -m 0755 central-sync /usr/local/bin/central-sync
install -o root -g root -m 0644 deployment/systemd/central-sync@.service /etc/systemd/system/
install -o root -g root -m 0644 deployment/systemd/central-sync@.timer /etc/systemd/system/
install -o root -g root -m 0644 deployment/logrotate/central-sync /etc/logrotate.d/central-sync
systemctl daemon-reload
/usr/local/bin/central-sync version
```

## 2. Préparer un projet

Dans cet exemple, `projet-1` est le nom local de l'instance, indépendant de l'ID
ODK Central. Choisissez un nom simple en minuscules, chiffres et tirets. Utilisez
exactement un projet Central par fichier YAML pour conserver des logs séparés.
Pour ajouter un autre projet, répétez cette section avec un autre nom.

```sh
install -d -o root -g central-sync -m 0750 /etc/central-sync
install -d -o root -g central-sync -m 0750 /etc/central-sync/projet-1
install -d -o central-sync -g central-sync -m 0750 /var/lib/central-sync
install -d -o central-sync -g central-sync -m 0750 /var/lib/central-sync/projet-1
install -d -o central-sync -g central-sync -m 0750 /var/log/central-sync
install -d -o central-sync -g central-sync -m 0750 /var/log/central-sync/projet-1
install -o root -g central-sync -m 0640 deployment/examples/project.env.example /etc/central-sync/projet-1/.env
install -o root -g central-sync -m 0640 deployment/examples/central_config.yaml.example /etc/central-sync/projet-1/central_config.yaml
ln -s /etc/central-sync/projet-1/.env /var/lib/central-sync/projet-1/.env
ln -s /etc/central-sync/projet-1/central_config.yaml /var/lib/central-sync/projet-1/central_config.yaml
ln -s /var/log/central-sync/projet-1/central-sync.log /var/lib/central-sync/projet-1/central-sync.log
```

Ces commandes créent une nouvelle instance. Ne recopiez pas les exemples sur une
configuration existante lors d'une mise à jour. Éditez les deux fichiers sous
`/etc/central-sync/projet-1/` : identifiants, ID du projet, base, datasets et
formulaires. Le programme lit lui-même `.env` ; ce n'est pas un script shell.
Les configurations restent détenues par root et lisibles par `central-sync`.

Initialisez la base PostgreSQL selon le README principal, ou appliquez les
migrations correspondant à la version installée sur une base existante. La base
et les permissions doivent être prêtes avant le premier lancement.

Avec `local_directory: attachments`, les pièces jointes sont stockées sous
`/var/lib/central-sync/projet-1/attachments/`. Le service autorise l'écriture dans
ses répertoires d'état et de logs ; conservez les fichiers dans ces chemins.
Le cache `central_session.json` est créé dans le répertoire d'état avec le mode
`0600`.

## 3. Tester puis activer le lancement horaire

```sh
systemd-analyze verify /etc/systemd/system/central-sync@.service /etc/systemd/system/central-sync@.timer
logrotate --debug /etc/logrotate.d/central-sync
systemctl start central-sync@projet-1.service
journalctl -u central-sync@projet-1.service -n 100 --no-pager
tail -n 100 /var/log/central-sync/projet-1/central-sync.log
```

Vérifiez les logs et `central_metadata.sync_runs` avant d'activer le timer.
Actuellement, certaines erreurs sont journalisées sans code de sortie non nul :
un service affiché comme réussi ne garantit pas une synchronisation réussie.
Le statut systemd seul ne doit donc pas servir d'alerte métier.

```sh
systemctl enable --now central-sync@projet-1.timer
systemctl list-timers 'central-sync@*'
systemctl status logrotate.timer
```

Le timer lance le projet à chaque heure pleine, selon le fuseau du VPS, avec
une tolérance d'une minute. `Persistent=true` déclenche un rattrapage si au moins
une échéance a été manquée pendant l'arrêt du timer ou du VPS ; il ne rejoue pas
chaque heure manquée. Une instance déjà en cours n'est pas lancée une seconde
fois. Le verrou PostgreSQL reste actif pour les autres lancements visant la même
base : utilisez des bases distinctes pour les instances qui doivent s'exécuter
simultanément. Aucun timeout global n'interrompt une longue synchronisation.

### Personnaliser le calendrier d'un projet

Pour remplacer le lancement horaire d'une instance, créez une surcharge :

```sh
systemctl edit central-sync@projet-1.timer
```

Par exemple, pour lancer ce projet à la minute 15 de chaque heure :

```ini
[Timer]
OnCalendar=
OnCalendar=*-*-* *:15:00
```

Systemd cumule les directives `OnCalendar`. La première ligne, avec une valeur
vide, efface les horaires hérités du modèle, notamment `hourly`. La deuxième
définit le nouvel horaire. Sans remise à zéro, les deux horaires seraient actifs.
Cette surcharge est propre au projet et reste en place lorsque l'installeur
réinstalle le modèle partagé.

Autres expressions possibles :

| Expression | Lancement |
| --- | --- |
| `*-*-* 08,12,18:00:00` | Tous les jours à 8 h, 12 h et 18 h |
| `Mon..Fri *-*-* 08:00:00` | Du lundi au vendredi à 8 h |

Validez l'expression puis rechargez et redémarrez le timer :

```sh
systemd-analyze calendar '*-*-* *:15:00'
systemctl daemon-reload
systemctl restart central-sync@projet-1.timer
systemctl list-timers 'central-sync@*'
```

Le fuseau du VPS, `Persistent=true` et la tolérance d'une minute restent
applicables. Le cumul et la remise à zéro sont décrits dans la section
`OnCalendar` de [systemd.timer](https://manpages.debian.org/trixie/systemd/systemd.timer.5.en.html).

## 4. Consulter les logs et arrêter la planification

```sh
journalctl -u central-sync@projet-1.service --since today
tail -f /var/log/central-sync/projet-1/central-sync.log
systemctl disable --now central-sync@projet-1.timer
```

Arrêter le timer laisse une exécution en cours se terminer. Les logs fichiers
sont tournés quotidiennement par logrotate, avec 14 archives conservées et une
compression différée d'une rotation. La rotation renomme le fichier : une
exécution en cours continue d'écrire dans l'archive, puis le prochain lancement
ouvre le nouveau fichier. Aucun `copytruncate` ni redémarrage du service n'est
nécessaire. Pour une exécution dépassant une journée, suivez aussi le journal
systemd : la compression différée n'est pas une garantie pour une exécution
encore ouverte lors de rotations ultérieures. La rétention de journald est
indépendante et gérée par la configuration du VPS.

## 5. Mettre à jour et sauvegarder

Désactivez les timers concernés et attendez la fin de leurs services
(`systemctl is-active central-sync@projet-1.service`). Sauvegardez les bases,
les configurations et les pièces jointes, puis appliquez les migrations requises.
Installez le nouveau binaire sans écraser celui qu'une exécution utilise :

```sh
install -o root -g root -m 0755 central-sync /usr/local/bin/central-sync.new
mv /usr/local/bin/central-sync.new /usr/local/bin/central-sync
/usr/local/bin/central-sync version
systemctl start central-sync@projet-1.service
```

Vérifiez la synchronisation puis réactivez les timers. Tous les projets utilisent
la même version du binaire. Si les modèles systemd ou logrotate ont changé,
réinstallez-les puis exécutez `systemctl daemon-reload` avant de réactiver les
timers. Sauvegardez `/etc/central-sync/`, les pièces jointes sous
`/var/lib/central-sync/` et les bases PostgreSQL ; protégez les sauvegardes qui
contiennent les identifiants et les caches de session.

Références Debian 13 : [systemd.timer](https://manpages.debian.org/trixie/systemd/systemd.timer.5.en.html),
[systemd.exec](https://manpages.debian.org/trixie/systemd/systemd.exec.5.en.html),
[logrotate](https://manpages.debian.org/trixie/logrotate/logrotate.8.en.html).
