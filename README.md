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

## Démarrage sans modèle

Go 1.22 ou ultérieur. Aucune dépendance Go externe.

```sh
go run ./cmd/aventure -offline
```

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

`OLLAMA_MODEL` et `OLLAMA_HOST` fournissent les valeurs par défaut des options correspondantes. `OLLAMA_HOST` doit contenir une URL complète avec `http://` ou `https://`. Le scénario par défaut est embarqué dans le binaire et fonctionne depuis n'importe quel répertoire. `-no-narration` conserve l'interprétation LLM tout en évitant le deuxième appel.

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

Le jeu lance le serveur sur `127.0.0.1:8080`, avec un contexte de 4096 tokens, un seul slot et l'alias `lfm3:1.7b`. Il attend que le modèle soit chargé. Le protocole utilise `/v1/chat/completions` et un schéma JSON dans `response_format`. La validation Go des actions reste identique.

Pour réutiliser un serveur déjà lancé, le fichier GGUF et le binaire ne sont pas requis par le jeu. `-model` doit correspondre à l'identifiant annoncé par `/v1/models` ou à l'alias configuré sur ce serveur :

```sh
go run ./cmd/aventure -provider llamacpp -auto-start=false \
  -llama-url http://127.0.0.1:8080 -model lfm3:1.7b
```

Il s'agit d'un wrapper de processus et d'un adaptateur HTTP, sans CGO. Une distribution peut regrouper `aventure`, `runtime/llama-server` et `models/lfm3-1.7b.gguf`, ainsi que les éventuelles bibliothèques natives requises par le binaire llama.cpp choisi. Le binaire compilé est présent localement mais ignoré par Git, comme les poids du modèle.

## Gestion du service et délais

| Option | Défaut | Rôle |
| --- | --- | --- |
| `-lang` | `en` | Langue du jeu : `en` ou `fr` |
| `-provider` | `ollama` | Choisir `ollama` ou `llamacpp` |
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

Pendant la partie, une panne d'interprétation n'applique aucune action ; une panne de narration conserve l'action validée. Utiliser les commandes `/look`, `/take key`, etc. pour continuer sans modèle. Un tour peut comporter deux appels LLM. Ctrl+C annule les requêtes et ferme le service possédé.

L'arrêt des groupes de processus et de leurs descendants est implémenté pour macOS/Linux, avec SIGTERM puis SIGKILL après trois secondes si nécessaire. Sur les autres plateformes, seul le processus enfant direct est arrêté ; la gestion complète des descendants Windows reste à ajouter. Un arrêt brutal du jeu par SIGKILL ne permet pas le nettoyage normal.

Le processus llama.cpp géré reçoit des options explicites et `--offline`. Il n'hérite pas des variables `LLAMA_*` du parent, afin de ne pas activer une autre configuration de modèle, d'authentification ou de serveur. Pour une configuration avancée, lancer le serveur soi-même puis utiliser `-auto-start=false`.

## Commandes locales et sauvegarde

| Anglais | Français | Effet |
| --- | --- | --- |
| `help` | `aide` | Afficher la grammaire et les commandes |
| `save` | `sauver` | Écrire l'état dans le fichier choisi avec `-save` |
| `load` | `charger` | Restaurer la dernière sauvegarde compatible |
| `quit` | `quitter` | Quitter sans sauvegarde automatique |
| `/inventory` | `/inventaire` | Consulter les possessions sans inférence |

Les commandes système acceptent aussi le préfixe `/`. En mode français, les alias anglais `help`, `save`, `load` et `quit` restent disponibles.

Les fichiers par défaut sont `savegame-en.json` et `savegame-fr.json`. Les variantes du scénario ont des identifiants et empreintes distincts : reprendre une partie avec la même langue. Le moteur refuse un chargement entre les deux variantes, sans modifier l'état courant. `-save` permet de choisir un autre fichier, notamment `-lang fr -load -save sauvegarde.json` pour une sauvegarde de l'ancien scénario français inchangé. Aucun ancien fichier n'est déplacé ni modifié automatiquement.

Le fichier de sauvegarde est du JSON lisible : version, identifiant du scénario, empreinte SHA-256 et état. Le moteur refuse les objets manquants, positions inconnues, objets fixes déplacés, drapeaux incohérents et versions incompatibles. Un chargement échoué laisse la partie courante intacte. Une sauvegarde réussie remplace l'ancienne ; son répertoire doit déjà exister.

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
internal/locale/     Langue choisie, messages traduits et erreurs avec leurs causes
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

Les tests couvrent les parcours français et anglais, le choix des prompts, les erreurs localisées et leurs causes, les règles du jeu, les refus sans mutation, les JSON invalides, la persistance, les deux protocoles HTTP et la boucle terminal. Des sous-processus de test vérifient le lancement, la disponibilité, l'arrêt gracieux, l'arrêt forcé, les descendants, l'annulation et les échecs de démarrage. Deux tests d'intégration lancent des serveurs simulés, jouent via les adaptateurs réels puis vérifient la fermeture du port. Ils ne nécessitent ni Ollama, ni llama.cpp, ni modèle, ni Internet. Le bac à sable doit autoriser les ports de boucle locale et les sous-processus.

Un essai manuel avec le GGUF `LFM3-1.7B-Q8_0`, llama.cpp v0.8.0 et Metal a terminé le scénario en français et en anglais : huit actions modifiant l'état, refus de la porte verrouillée, narration, sauvegarde, rechargement et arrêt du serveur possédé. Les directions exactes sont natives ; les prises et utilisations d'objets de ces parcours sont interprétées par le modèle. Cet essai ne constitue pas une comparaison générale de qualité entre langues.

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
