# AI Venture — The Last Lighthouse

Jeu d'aventure textuelle en anglais ou français. Les scénarios et traductions sont embarqués dans l'exécutable. Go n'est pas nécessaire pour jouer.

Cette archive cible le système et l'architecture indiqués dans son nom et dans `BUILD.json`. Extraire tout le dossier dans un emplacement accessible en écriture, puis ouvrir un terminal dans ce dossier. Les sauvegardes et le cache SQLite sont créés dans le répertoire courant.

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

- `-lang en` ou `-lang fr` : langue de la partie.
- `-color auto|always|never` : couleurs ANSI, automatiques par défaut.
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
