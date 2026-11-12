# AI Venture — The Last Lighthouse

Jeu d'aventure textuelle illustrée en anglais ou français, dans une fenêtre graphique indépendante. Les traductions et le scénario de base du phare sont embarqués dans l'exécutable ; les fichiers Providence et son catalogue source sont inclus dans le paquet. Go n'est pas nécessaire pour jouer.

Cette archive cible le système et l'architecture indiqués dans son nom et dans `BUILD.json`. Sur macOS, double-cliquer sur **AI Venture.app** pour jouer immédiatement à **Providence — The Threshold**, en anglais et hors ligne. Ses données sont conservées dans `~/Library/Application Support/ai-venture`, hors de l'application, avec une base et des sauvegardes portant le nom `providence`.

Pour les autres systèmes ou les options LLM, extraire tout le dossier dans un emplacement accessible en écriture, puis utiliser les commandes ci-dessous. La fenêtre s'ouvre automatiquement ; ajouter `-ui terminal` pour jouer dans un terminal. Les sauvegardes et le cache SQLite sont créés dans le répertoire courant, ou dans `-data-dir dossier`. Voir [docs/window.md](docs/window.md) pour les commandes de la fenêtre. Sous Linux, un bureau X11 ou compatible XWayland et un pilote graphique sont nécessaires à la fenêtre.

## Jouer sans modèle

```sh
./aventure -offline -lang fr
./aventure -offline -lang en
```

Sous Windows, utiliser `aventure.exe` à la place de `./aventure`.

## Jouer avec un LLM local

Le jeu peut démarrer le `runtime/llama-server` fourni dans l'archive standard. Placer un modèle GGUF dans le dossier `models`, puis adapter son nom dans la commande :

```sh
./aventure -provider llamacpp -gguf ./models/modele.gguf -lang fr
```

Les poids du modèle sont à fournir séparément. Aucun modèle n'est téléchargé automatiquement. Le serveur utilise Metal sur macOS et le processeur sur Linux. Il cible la même architecture que le jeu et dépend des bibliothèques et frameworks du système ; aucune bibliothèque privée du workspace n'est requise.

Les archives dont le nom se termine par `game-only` contiennent uniquement le jeu et ses notices. Avec cette variante, installer Ollama ou fournir un `llama-server` séparément :

```sh
./aventure -model lfm3:1.7b -lang fr
./aventure -provider llamacpp -llama-bin /chemin/llama-server -gguf ./models/modele.gguf
./aventure -provider llamacpp -auto-start=false -model alias-du-serveur
```

Pour Ollama, le modèle choisi doit être déjà installé. Un serveur existant reste lancé à la sortie du jeu ; un serveur démarré par le jeu est arrêté à sa fermeture.

## Commandes utiles

Pour essayer les images composées par Go, sans modèle :

```sh
./catalogue import -db catalogue.sqlite -source data/catalogue/providence
./aventure -offline -lang fr -scenario scenarios/providence-fr.json -catalogue catalogue.sqlite -save savegame-providence-fr.json -visuals
```

Les PNG s'affichent dans la fenêtre et restent exportés dans `<sauvegarde>.visuals`. Avec `-ui terminal`, leurs chemins sont affichés. Voir `docs/catalogue.md`. L'exécutable `catalogue` est fourni ; utiliser `catalogue.exe` sous Windows.

- `-lang en` ou `-lang fr` : langue de la partie.
- `-ui window|terminal` : fenêtre indépendante par défaut, terminal en option.
- `-data-dir dossier` : dossier des données générées (sauf chemins explicitement fournis).
- `-color auto|always|never` : couleurs ANSI du mode terminal, automatiques par défaut.
- `-catalogue catalogue.sqlite -visuals` : fiches importées et scènes PNG composées par Go.
- `-visual-dir dossier` : emplacement des PNG exportés.
- `-art` : illustrations LLM des lieux en ASCII/ANSI, au maximum 80 colonnes et 24 lignes. `art` / `dessin` réaffiche le dessin courant ; un petit modèle peut produire des dessins très sommaires.
- `-save ma-partie.json` et `-load` : emplacement de sauvegarde et reprise au démarrage.
- `-rag=false` : désactiver la mémoire documentaire SQLite de la narration.
- `-llama-url http://127.0.0.1:8081` : utiliser un autre port local si nécessaire.
- `-help` : afficher toutes les options.

Dans le jeu, saisir `help` / `aide` pour les commandes. Le préfixe `/` permet les commandes locales : `/look`, `/take key`, `/inventory` en anglais ; `/regarder`, `/prendre cle`, `/inventaire` en français.

## Contenu

`BUILD.json` indique la version, les dépendances et l'empreinte SHA-256 de l'exécutable. Le runtime fourni possède aussi son manifeste dans `runtime/llama-server.build.json`. Les licences et notices des dépendances figurent dans `THIRD_PARTY_NOTICES.md`, `licenses/` et `runtime/llama.cpp.LICENSE` quand le runtime est inclus.

Le fichier `.tar.gz.sha256` fourni à côté de l'archive permet de contrôler le téléchargement :

```sh
shasum -a 256 -c ai-venture-VERSION-SYSTEME-ARCHITECTURE.tar.gz.sha256
```

Sous Linux, `sha256sum -c` peut remplacer `shasum -a 256 -c`.

L'application macOS contient aussi ses licences et son manifeste dans `Contents/Resources`. Elle n'est pas encore signée Developer ID ou notarisée. Pour jouer en français avec la démonstration intégrée :

```sh
"AI Venture.app/Contents/MacOS/aventure" -offline -lang fr
```
