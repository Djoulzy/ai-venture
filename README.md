# AI Venture — The Last Lighthouse

Prototype de jeu d'aventure textuelle en Go, en anglais ou en français, inspiré des années 1980–1990. LFM3 interprète les commandes et enrichit la narration via Ollama ou llama.cpp ; le moteur Go décide seul des changements d'état. Le jeu peut démarrer et arrêter son service d'inférence local.

La langue se choisit au lancement avec `-lang en` (défaut) ou `-lang fr`, avec ou sans LLM. Ce choix sélectionne les prompts d'interprétation et de narration, les messages du terminal, la grammaire locale et la variante du scénario.

```sh
go run ./cmd/aventure -lang en
go run ./cmd/aventure -lang fr
go run ./cmd/aventure -offline -lang fr
```

Avec le GGUF présent dans ce workspace :

```sh
go run ./cmd/aventure -provider llamacpp -gguf ./models/LFM3-1.7B-Q8_0.gguf -lang en
# Remplacer en par fr pour jouer en français.
```

Le scénario embarqué propose quatre lieux, des objets transportables, une porte verrouillée et une balise à rallumer. Ses variantes sont définies dans [scenarios/lighthouse.json](scenarios/lighthouse.json) (anglais) et [scenarios/phare.json](scenarios/phare.json) (français). Un fichier fourni avec `-scenario` conserve ses propres textes et identifiants : le jeu ne le traduit pas automatiquement.

Les sorties utilisent des couleurs ANSI : lieux et sorties en cyan, objets en jaune, faits et sauvegardes réussies en vert, narration en magenta, avertissements en jaune et erreurs en rouge. Les descriptions restent dans la couleur habituelle du terminal.

`-color auto` est le défaut : les couleurs sont activées sur un terminal et désactivées pour les fichiers et les pipes, avec `NO_COLOR` non vide ou avec `TERM=dumb`. `-color always` les force ; `-color never` les désactive. Ces options fonctionnent aussi en mode hors ligne et dans les deux langues.

```sh
go run ./cmd/aventure -offline -lang fr
go run ./cmd/aventure -color never
go run ./cmd/aventure -offline -color always
```

## Démarrage sans modèle

Go **1.26 ou ultérieur**. Le pilote SQLite `modernc.org/sqlite` est figé dans `go.mod` ; il fonctionne sans CGO. Le premier build télécharge les dépendances Go, puis le jeu et la recherche fonctionnent localement.

```sh
go run ./cmd/aventure -offline
```

## Compiler et distribuer

Le Makefile fournit un build Go sans CGO, avec chemins de développement retirés et symboles de débogage supprimés :

```sh
make                    # bin/aventure (aventure.exe pour Windows)
make check              # tests, détection de courses concurrentes et go vet
make dist VERSION=0.1.0  # archive complète pour la machine qui compile
```

`make dist` compile le jeu puis le runtime llama.cpp avec le script existant. Il exige donc aussi les outils natifs décrits dans [runtime/README.md](runtime/README.md) et Python 3. Le premier lancement peut télécharger les dépendances Go, les sources llama.cpp et CMake. Les poids GGUF et les données de partie ne sont jamais copiés dans l'archive.

Le résultat est `dist/ai-venture-0.1.0-darwin-arm64.tar.gz` sur un Mac ARM64, accompagné de son fichier `.sha256`. Le dossier archivé contient `aventure`, `runtime/llama-server`, ses métadonnées, un dossier `models/` à compléter, les licences et notices des dépendances Go/natives, un manifeste `BUILD.json` et un guide de lancement. Les scénarios FR/EN et SQLite sont embarqués dans le jeu ; le destinataire n'a pas besoin de Go. Le runtime utilise les bibliothèques du système cible. Le guide inclus est [docs/distribution.md](docs/distribution.md).

Pour distribuer uniquement le jeu, sans compiler ni inclure llama-server :

```sh
make dist WITH_LLAMA=0 VERSION=0.1.0
make dist WITH_LLAMA=0 VERSION=0.1.0 GOOS=linux GOARCH=amd64
make dist WITH_LLAMA=0 VERSION=0.1.0 GOOS=windows GOARCH=amd64
```

Ces archives portent le suffixe `-game-only` et permettent le mode hors ligne, Ollama ou un llama-server fourni séparément. Le bundle complet doit être compilé nativement sur chaque système et architecture cibles ; le Makefile refuse d'y mélanger un runtime de la machine de compilation avec un exécutable Go destiné à une autre plateforme. Les archives sont reconstruites depuis une sélection explicite de fichiers, avec métadonnées tar normalisées et publication seulement après création complète.

`VERSION` vaut `dev` par défaut. `make help` affiche les cibles et `make clean` supprime les binaires Go et `dist/`, en conservant modèles, sauvegardes, runtime natif et cache de compilation llama.cpp.

Ce mode utilise une grammaire anglaise limitée : `look`, `examine key`, `take key`, `drop key`, `go north`, `inventory`, `use key on door`. Avec `-lang fr`, utiliser `regarder`, `observer cle`, `prendre cle`, `deposer cle`, `aller nord`, `inventaire`, `utiliser cle sur porte`. Les noms complets affichés sont également reconnus. Les directions exactes affichées (`east`, `north`, `est`, `nord`, etc., selon le scénario) sont des raccourcis traités directement par Go, même en mode LLM ; les phrases comme « I head towards the workshop » sont interprétées par le modèle. Le langage naturel libre nécessite le mode LLM.

## Avec LFM3 via Ollama

Installer [Ollama](https://ollama.com/download) et préparer le modèle une fois, avec le service Ollama actif :

```sh
ollama pull lfm3:1.7b
```

Pour ce téléchargement initial, si le service n'est pas actif, ouvrir l'application Ollama ou lancer `ollama serve` dans un autre terminal. Les parties suivantes se lancent avec une seule commande :

```sh
go run ./cmd/aventure
```

Le jeu réutilise le service existant ou lance `ollama serve` s'il est absent, puis vérifie la présence du modèle. Il arrête uniquement le service qu'il a démarré. Le binaire `ollama` doit être dans le `PATH`, ou être indiqué avec `-ollama-bin /chemin/vers/ollama`. Aucun téléchargement de modèle n'est déclenché par le jeu.

Exemples : « I pick up the copper key », « I head towards the workshop », « Unlock the door with the key ». Demander une seule action par saisie. Le terminal affiche les faits du moteur, puis une courte ambiance produite par le modèle.

Le tag par défaut est celui du [catalogue officiel LFM3](https://ollama.com/library/lfm3:1.7b). L'adaptateur utilise [`POST /api/chat`](https://docs.ollama.com/api/chat), `stream: false`, `think: false` et un [JSON Schema dans `format`](https://docs.ollama.com/capabilities/structured-outputs). Chaque réponse est validée de nouveau en Go.

```sh
go run ./cmd/aventure -model lfm3:1.7b -ollama-url http://localhost:11434 -timeout 90s
go run ./cmd/aventure -no-narration
go run ./cmd/aventure -auto-start=false
go run ./cmd/aventure -scenario scenarios/lighthouse.json -save ma-partie.json
go run ./cmd/aventure -load -save ma-partie.json
go build -o bin/aventure ./cmd/aventure
```

`OLLAMA_MODEL` et `OLLAMA_HOST` fournissent les valeurs par défaut pour le fournisseur Ollama uniquement. `OLLAMA_HOST` doit contenir une URL complète avec `http://` ou `https://`. Le scénario par défaut est embarqué dans le binaire et fonctionne depuis n'importe quel répertoire. `-no-narration` conserve l'interprétation LLM tout en évitant le deuxième appel.

## Avec llama.cpp et un fichier GGUF

Le workspace intègre une compilation reproductible de llama.cpp **v0.8.0** (support de LFM3), à partir du dépôt officiel. Pour construire le serveur local puis lancer le jeu avec un fichier GGUF existant :

```sh
./scripts/build-llama-server.sh
go run ./cmd/aventure -provider llamacpp -gguf ./models/lfm3-1.7b.gguf
```

Le jeu détecte automatiquement `runtime/llama-server`. Voir [runtime/README.md](runtime/README.md) pour les prérequis, l'organisation des sources et les options de compilation. Le modèle GGUF reste à fournir séparément.

Il est également possible de fournir un autre binaire compatible avec LFM3 :

```sh
go run ./cmd/aventure -provider llamacpp \
  -llama-bin ./runtime/llama-server \
  -gguf ./models/lfm3-1.7b.gguf
```

Le jeu lance le serveur sur `127.0.0.1:8080`, avec un contexte de 4096 tokens et un seul slot. L'alias et le nom affiché sont dérivés du nom du fichier GGUF, sauf si `-model` fournit un alias explicite. Par exemple, cette commande sélectionne Llama 3.2 et affiche `llama-3.2-3b-instruct-q4_k_m.gguf` :

```sh
go run ./cmd/aventure -lang en -provider llamacpp \
  -gguf ./models/llama-3.2-3b-instruct-q4_k_m.gguf
```

Le jeu attend le chargement du modèle et vérifie le chemin annoncé par `/props` avant d'afficher `Verified GGUF`. Cette vérification porte aussi sur les serveurs existants : un alias identique ne suffit pas à accepter un autre fichier. Un serveur utilisant un autre modèle est laissé intact et la partie s'arrête avec une erreur explicite. Utiliser par exemple `-llama-url http://127.0.0.1:8081` pour démarrer le modèle demandé sur un autre port libre.

Le protocole utilise `/v1/chat/completions`, avec `response_format.type=json_schema` et le schéma imbriqué dans `response_format.json_schema.schema`. La validation Go des actions reste identique.

Pour réutiliser un serveur déjà lancé, le fichier GGUF et le binaire ne sont pas requis par le jeu. `-model` doit correspondre à l'identifiant annoncé par `/v1/models` ou à l'alias configuré sur ce serveur :

```sh
go run ./cmd/aventure -provider llamacpp -auto-start=false \
  -llama-url http://127.0.0.1:8080 -model lfm3:1.7b
```

Sans `-gguf`, le choix repose sur `-model`, qui est alors obligatoire pour llama.cpp. Avec `-gguf`, le serveur doit exposer un chemin absolu dans `/props` ; les liens locaux vers le même fichier sont acceptés. Les serveurs distants ou conteneurisés dont les chemins diffèrent se sélectionnent avec `-model` sans `-gguf`. Le jeu compare les chemins annoncés, sans calculer l'empreinte des poids en mémoire.

Il s'agit d'un wrapper de processus et d'un adaptateur HTTP, sans CGO. Une distribution peut regrouper `aventure`, `runtime/llama-server` et `models/lfm3-1.7b.gguf`, ainsi que les éventuelles bibliothèques natives requises par le binaire llama.cpp choisi. Le binaire compilé est présent localement mais ignoré par Git, comme les poids du modèle.

## Gestion du service et délais

| Option | Défaut | Rôle |
| --- | --- | --- |
| `-rag` | `true` | Enrichir la narration par une recherche SQLite FTS5 |
| `-rag-db` | `<save>.rag.sqlite` | Chemin du cache documentaire local |
| `-lang` | `en` | Langue du jeu : `en` ou `fr` |
| `-color` | `auto` | Couleurs ANSI : `auto`, `always` ou `never` |
| `-art` | `false` | Illustrations LLM des lieux, au maximum 80 colonnes et 24 lignes |
| `-provider` | `ollama` | Choisir `ollama` ou `llamacpp` |
| `-model` | Ollama : `OLLAMA_MODEL` ou `lfm3:1.7b` ; llama.cpp : nom du GGUF | Modèle Ollama ou alias du serveur llama.cpp |
| `-auto-start` | `true` | Démarrer le service local uniquement si la connexion est refusée |
| `-startup-timeout` | `2m` | Attente maximale de disponibilité au démarrage |
| `-timeout` | `45s` | Délai maximal par appel HTTP/LLM |
| `-ollama-bin` | `ollama` | Exécutable utilisé pour lancer Ollama |
| `-llama-bin` | Runtime local, sinon `llama-server` | Exécutable utilisé pour lancer llama.cpp |
| `-llama-url` | `http://127.0.0.1:8080` | Racine du serveur, sans suffixe `/v1` |
| `-gguf` | vide | Fichier local requis pour lancer llama.cpp |
| `-llama-context` | `4096` | Contexte du serveur llama.cpp lancé par le jeu |

Le lancement automatique est limité aux URL HTTP de boucle locale sans préfixe de chemin. Un serveur déjà actif, y compris en cours de chargement, est réutilisé sans être arrêté à la sortie. Un modèle absent ou un service incompatible produit une erreur avant la partie ; `-offline` permet de jouer sans inférence. Les dernières lignes du service sont conservées en mémoire, dans une limite de 8 Kio, pour diagnostiquer les erreurs de démarrage.

Pour un Ollama lancé par le jeu, le modèle reste chargé après le premier appel jusqu'à l'arrêt du service. Le premier appel effectue le chargement en mémoire : augmenter `-timeout` si nécessaire. Un service Ollama réutilisé garde sa propre politique de conservation du modèle. Avec llama.cpp, le chargement initial est inclus dans `-startup-timeout`.

Pendant la partie, une panne d'interprétation n'applique aucune action ; une panne de narration conserve l'action validée. Utiliser les commandes `/look`, `/take key`, etc. pour continuer sans modèle. Un tour peut comporter deux appels LLM, ou trois si `-art` nécessite un nouveau dessin. Ctrl+C annule les requêtes et ferme le service possédé.

L'arrêt des groupes de processus et de leurs descendants est implémenté pour macOS/Linux, avec SIGTERM puis SIGKILL après trois secondes si nécessaire. Sur les autres plateformes, seul le processus enfant direct est arrêté ; la gestion complète des descendants Windows reste à ajouter. Un arrêt brutal du jeu par SIGKILL ne permet pas le nettoyage normal.

Le processus llama.cpp géré reçoit des options explicites et `--offline`. Il n'hérite pas des variables `LLAMA_*` du parent, afin de ne pas activer une autre configuration de modèle, d'authentification ou de serveur. Pour une configuration avancée, lancer le serveur soi-même puis utiliser `-auto-start=false`.

## Mémoire documentaire locale (RAG)

Le RAG est activé par défaut avec la narration LLM, pour Ollama comme pour llama.cpp. Le jeu crée `savegame-en.json.rag.sqlite` ou `savegame-fr.json.rag.sqlite` à côté de la sauvegarde choisie. Le dossier doit exister. Aucun modèle d'embeddings ni service supplémentaire n'est requis.

```sh
go run ./cmd/aventure -provider llamacpp -gguf ./models/LFM3-1.7B-Q8_0.gguf -lang fr
go run ./cmd/aventure -rag-db ./memory.rag.sqlite
go run ./cmd/aventure -rag=false
```

SQLite utilise FTS5, un tokenizer Unicode insensible aux accents et un classement BM25. La recherche est lexicale : les synonymes et les traductions ne sont pas rapprochés automatiquement. Elle s'appuie sur le résultat validé et le lieu courant ; elle transmet au narrateur au maximum trois extraits, avec un budget de 1 600 caractères Unicode pour les titres et textes. Les conditions de découverte et les règles ne sont jamais transmises. L'interpréteur de commandes reçoit toujours uniquement la vue visible.

Go conserve les documents découverts et les 128 dernières actions de mutation réussies. Les observations, les actions refusées et les textes produits par le modèle ne sont pas ajoutés au journal. Les passages documentaires sont découverts automatiquement lorsque leurs exigences sont réunies, puis restent connus. Chaque recherche remplace transactionnellement le corpus du scénario et de la langue par la copie autorisée du moteur. Une nouvelle partie ou un chargement ne peut donc pas réutiliser les souvenirs d'une branche abandonnée. Le cache sur disque peut refléter la dernière recherche jusqu'à la suivante ; il n'est jamais chargé comme état de partie.

Si le cache manque, il est recréé. En cas d'erreur SQLite ou de délai dépassé, la narration continue avec les faits du moteur. `-offline`, `-no-narration` et `-rag=false` n'ouvrent pas SQLite ; les découvertes et le journal Go restent sauvegardés. Les fichiers SQLite peuvent être supprimés quand le jeu est arrêté sans perdre la progression.

Les scénarios de démonstration contiennent chacun cinq documents, dont une inscription découverte après ouverture de la porte. Pour enrichir un scénario externe, ajouter le champ facultatif `documents` :

```json
{
  "id": "stair_inscription",
  "language": "en",
  "title": "An inscription inside the copper door",
  "text": "The inscription dedicates the stairs to the island rescue crews.",
  "location": "hall",
  "item": "door",
  "conditions": [{"flag": "door_open", "value": true}]
}
```

Les contraintes `location`, `item` et `conditions` sont facultatives et se cumulent. Sans contrainte, le document est public dès le départ. Une référence d'objet exige qu'il soit visible ou porté au moment de la découverte. `language` vaut `en` ou `fr`. Le validateur accepte jusqu'à 512 documents par scénario, avec des titres de 120 caractères et des textes de 2 000 caractères au maximum. Les futurs générateurs passent par cette même validation.

## Illustrations ASCII / ANSI des lieux

`-art` active un illustrateur LLM indépendant de la narration, disponible avec Ollama ou llama.cpp :

```sh
go run ./cmd/aventure -lang en -provider llamacpp \
  -gguf ./models/llama-3.2-3b-instruct-q4_k_m.gguf -art
```

Le dessin comporte **au maximum 80 colonnes et 24 lignes**, espaces compris. Cette limite concerne le dessin, sans bordure ni légende ajoutées ; la description et les commandes continuent de défiler dans le terminal. Prévoir un terminal d'au moins 80 colonnes pour éviter le repli visuel des lignes.

Le modèle reçoit uniquement le décor visible et renvoie un JSON `lines`, chaque ligne ayant un `text` ASCII imprimable et un `color` parmi `default`, `cyan`, `yellow`, `green`, `blue`, `magenta`, `red`, `gray`. Go valide le document entier avant de l'afficher : lignes trop longues ou trop nombreuses, Unicode, tabulations, séquences ANSI et champs inattendus sont refusés. Le moteur ajoute les couleurs de sa propre palette ; `-color never` fournit le même dessin en ASCII simple. Les codes ANSI ne comptent pas dans les 80 colonnes visibles.

Une première illustration est demandée au démarrage, puis lors d'un déplacement ou d'une observation globale sans préfixe `/`. Le cache en mémoire réutilise un dessin tant que le décor visible n'a pas changé, conserve un dessin par lieu et au maximum 64 lieux. Les objets portés ne sont pas transmis. Les règles, drapeaux, documents RAG et événements passés ne sont pas fournis à l'illustrateur. Les dessins sont décoratifs : les faits canoniques du moteur restent la référence.

`art` en anglais, `dessin` en français, génère ou réaffiche l'illustration courante sans consommer de tour. `/art` et `/dessin` affichent uniquement un dessin déjà en cache ; comme les autres commandes préfixées, ils n'appellent jamais le LLM. Un chargement réussi vide le cache ; demander ensuite `art` / `dessin` pour illustrer le lieu restauré. Les dessins ne sont pas enregistrés dans les sauvegardes ou SQLite.

La génération ajoute un appel LLM de 2 048 tokens de sortie au maximum, soumis à `-timeout`. Sa qualité dépend du modèle ; les petits modèles peuvent produire des formes très sommaires malgré un format valide. Une réponse invalide ou une indisponibilité laisse la description textuelle disponible, sans mutation du monde. `-art=false` est le défaut ; `-offline` désactive aussi l'illustrateur. `-no-narration` coupe la prose, mais permet les dessins si `-art` est demandé explicitement.

## Commandes locales et sauvegarde

| Anglais | Français | Effet |
| --- | --- | --- |
| `help` | `aide` | Afficher la grammaire et les commandes |
| `save` | `sauver` | Écrire l'état dans le fichier choisi avec `-save` |
| `load` | `charger` | Restaurer la dernière sauvegarde compatible |
| `quit` | `quitter` | Quitter sans sauvegarde automatique |
| `/inventory` | `/inventaire` | Consulter les possessions sans inférence |

Les commandes système acceptent aussi le préfixe `/`. En mode français, les alias anglais `help`, `save`, `load` et `quit` restent disponibles.

Les fichiers par défaut sont `savegame-en.json` et `savegame-fr.json`. Les variantes du scénario ont des identifiants et empreintes distincts : reprendre une partie avec la même langue. Le moteur refuse un chargement entre les deux variantes, sans modifier l'état courant. `-save` permet de choisir un autre fichier, notamment `-lang fr -load -save sauvegarde.json` pour une ancienne sauvegarde française compatible. Aucun ancien fichier n'est déplacé ni modifié automatiquement.

Le fichier de sauvegarde est du JSON lisible : version, identifiant du scénario, empreinte SHA-256 et état. La version 2 inclut les identifiants des documents découverts et le journal typé ; elle ne stocke aucune narration générée. Les sauvegardes v1 sont reprises si le scénario correspond exactement, ou si seuls les nouveaux documents ont été ajoutés. Les faits passés absents de la sauvegarde ne sont pas inventés ; seules les découvertes accessibles au moment de la reprise sont recalculées. Un changement des règles ou des autres champs du scénario reste incompatible. Le moteur refuse les objets manquants, positions inconnues, objets fixes déplacés, drapeaux incohérents et versions incompatibles. Un chargement échoué laisse la partie courante intacte. Une sauvegarde réussie remplace l'ancienne ; son répertoire doit déjà exister.

L'empreinte assure la compatibilité avec le scénario, pas l'authenticité de la sauvegarde. Ce prototype ne cherche pas à empêcher un joueur d'éditer ses drapeaux localement. L'écriture est atomique contre un fichier partiel ; elle ne constitue pas une garantie de durabilité face à toutes les pannes matérielles.

## Architecture et contrats

Voir [docs/architecture.md](docs/architecture.md) pour le flux de données et les frontières de confiance, et [AGENTS.md](AGENTS.md) pour les conventions de développement.

```text
cmd/aventure/         Configuration et lancement
internal/world/      Définition, chargement et validation des scénarios
internal/engine/     Actions, règles, préconditions et état privé
internal/inference/  Interface de fournisseur LLM
internal/ollama/     Adaptateur HTTP Ollama
internal/llamacpp/   Adaptateur HTTP llama-server
internal/modelruntime/ Démarrage, disponibilité et arrêt des services possédés
internal/inference/httpjson/ Transport HTTP borné partagé
internal/language/   Prompts et grammaires français/anglais
internal/illustration/ Dessins LLM ASCII, palette et validation 80 x 24
internal/locale/     Langue choisie, messages traduits et erreurs avec leurs causes
internal/retrieval/  Contrat de recherche et budgets du contexte
internal/retrieval/sqlite/ Cache FTS5 transactionnel des souvenirs autorisés
internal/persistence/ Sauvegarde et chargement atomiques
internal/terminal/   Boucle de jeu, commandes locales et affichage
internal/strictjson/ Décodage JSON strict aux frontières
scenarios/           Scénario JSON embarqué
```

Exemple de proposition du modèle :

```json
{"action":"use","target":"door","tool":"key"}
```

Ce JSON ne contient aucun effet. Le moteur vérifie la présence de la porte, la possession de la clé, le lieu et les préconditions avant d'appliquer les effets déclarés dans le scénario. Il refuse les champs supplémentaires, propositions multiples, identifiants inaccessibles et actions inconnues. Les observations et l'inventaire ne consomment pas de tour ; les actions de mutation réussies en consomment un.

Les deux adaptateurs implémentent `inference.Client` ; la gestion des processus est séparée dans `modelruntime`. Les futurs générateurs de scénarios pourront implémenter `world.Source` ; une provenance avec graine est prévue. Tous les scénarios passent par `world.Validate`, puis sont copiés dans le moteur. La validation structurelle ne prouve pas encore qu'un scénario généré est résoluble : un solveur sera nécessaire avant cette étape. Cette version ne génère pas de scénarios et ne conserve pas d'historique conversationnel.

La narration ne modifie jamais l'état et n'est pas réinjectée dans l'interpréteur. Sa conformité factuelle ne peut pas être garantie par le seul JSON Schema : les faits Go et les sorties affichées sont la référence. Les tests automatisés utilisent des réponses simulées. La qualité d’interprétation et de prose dépend du modèle et ne se déduit pas de leur réussite.

## Vérification

```sh
go test ./...
go test -race ./...
go vet ./...
```

Les tests couvrent les parcours français et anglais, le choix des prompts, les erreurs localisées et leurs causes, la recherche SQLite réelle, les secrets, les branches abandonnées, les migrations v1, les règles du jeu, les refus sans mutation, les JSON invalides, la persistance, les deux protocoles HTTP et la boucle terminal. Des sous-processus de test vérifient le lancement, la disponibilité, l'arrêt gracieux, l'arrêt forcé, les descendants, l'annulation et les échecs de démarrage. Deux tests d'intégration lancent des serveurs simulés, jouent via les adaptateurs réels puis vérifient la fermeture du port. Ils ne nécessitent ni Ollama, ni llama.cpp, ni modèle, ni Internet. Le bac à sable doit autoriser les ports de boucle locale et les sous-processus.

Les illustrations sont testées aux limites exactes de 80 colonnes et 24 lignes, avec rejet des dépassements, contrôles, Unicode et JSON non conformes. Les tests vérifient aussi le rendu avec/sans ANSI, l'absence d'affichage partiel, le cache, son invalidation et le repli après erreur. Des essais réels avec Llama 3.2 3B en anglais et LFM3-1.7B en français ont produit des dessins valides, mais graphiquement sommaires ; la validité du format ne garantit pas une illustration reconnaissable ou fidèle.

Un essai manuel avec le GGUF `LFM3-1.7B-Q8_0`, llama.cpp v0.8.0 et Metal a terminé le scénario en français et en anglais : huit actions modifiant l'état, refus de la porte verrouillée, narration, sauvegarde, rechargement et arrêt du serveur possédé. Les directions exactes sont natives ; les prises et utilisations d'objets de ces parcours sont interprétées par le modèle. Cet essai ne constitue pas une comparaison générale de qualité entre langues.

Le même parcours a été vérifié avec le RAG activé dans les deux langues : cinq documents découverts et huit événements en fin de partie, remplacement du cache par les deux documents accessibles au départ lors d'une nouvelle partie, puis reconstruction des treize entrées au rechargement de la sauvegarde. Le schéma envoyé à llama-server est placé dans `response_format.json_schema.schema` afin de contraindre effectivement la forme des réponses.

Un essai avec `llama-3.2-3b-instruct-q4_k_m.gguf`, sans `-model`, a confirmé le fichier chargé via `/props` et les 3 212 749 888 paramètres annoncés par `/v1/models`. Trois commandes anglaises en langage naturel (prendre la clé, rejoindre le phare, ouvrir la porte), la narration, la sauvegarde et l'arrêt du serveur ont été vérifiés. Les tests simulés couvrent également le refus d'un serveur chargé avec un autre GGUF malgré un alias identique.

Certaines formulations libres restent mal interprétées, notamment lors d'une observation d'objet. Les commandes `/examine key` et `/observer cle` donnent accès à l'observation locale dans la langue correspondante ; les propositions invalides du modèle sont refusées sans modifier l'état.

Un test de fuzzing court est également disponible :

```sh
go test ./internal/language -fuzz=FuzzActionJSON -fuzztime=5s
```

<details>
<summary>Solution du scénario de démonstration</summary>

```text
take key
east
take battery
west
north
use key on door
up
use battery on beacon
save
quit
```

La batterie recharge la balise puis reste dans l'inventaire ; les effets de ce prototype sont des drapeaux booléens, sans consommation d'objet.

</details>
