# Fenêtre de jeu

`aventure` ouvre par défaut une fenêtre native indépendante, avec Ebitengine. Le moteur, les fournisseurs Ollama/llama.cpp, le catalogue et les sauvegardes restent communs aux deux interfaces. Ajouter `-ui terminal` pour utiliser l'interface historique, les scripts et les pipes.

```sh
go run ./cmd/aventure -offline -lang fr
go run ./cmd/aventure -lang en -provider llamacpp -gguf ./models/llama-3.2-3b-instruct-q4_k_m.gguf
go run ./cmd/aventure -ui terminal -offline -lang fr
```

Les options de langue et de modèle se choisissent au lancement. La fenêtre est redimensionnable ; sa taille minimale est de 1040 × 760. Elle présente la scène et ses objets à gauche, le journal déroulant à droite et la saisie en bas. La description complète reste accessible dans le journal lorsque sa légende ne suffit pas.

- Entrée ou **Valider / Send** soumet la commande. En mode LLM, la saisie accepte une intention formulée librement dans la langue choisie, par exemple « je déchiffre les annotations du livre ». Les commandes `/...` utilisent la grammaire locale. Les effets restent validés par le moteur selon le scénario.
- ↑ / ↓ parcourent les 64 dernières commandes ; Échap efface la saisie et annule le choix d'une cible. Début / Fin et ← / → déplacent le curseur.
- Cmd+A ou Ctrl+A sélectionne toute la saisie. Cmd+V / Ctrl+V colle sous macOS ; sous Linux, `wl-paste` ou `xclip` est nécessaire. Le collage Windows reste à ajouter.
- La molette et Page précédente / suivante font défiler le journal. L'onglet Inventaire permet d'observer, déposer ou utiliser un objet ; après **Utiliser**, choisir une cible ou compléter la commande proposée.
- Les boutons de sortie déplacent le joueur ; Regarder, Sauver, Charger et Aide utilisent les commandes locales. Ils passent par les mêmes validations que la saisie et ne consomment pas d'appel LLM pour l'interprétation.

Les commandes sont traitées une par une. La saisie peut être préparée pendant l'inférence ; sa validation et les actions sont désactivées jusqu'au prochain résultat. Une panne de démarrage reste visible dans le journal et propose **Jouer hors ligne**. Fermer la fenêtre annule les requêtes, débloque la saisie et arrête uniquement le serveur lancé par le jeu. La fermeture ne sauvegarde pas automatiquement : utiliser Sauver avant de quitter.

## Images du catalogue

```sh
./bin/catalogue import -db catalogue.sqlite -source data/catalogue/providence
./bin/aventure -offline -lang fr -scenario scenarios/providence-fr.json -catalogue catalogue.sqlite -save savegame-providence-fr.json
```

Dans la fenêtre, les visuels d'un scénario lié à un catalogue sont automatiquement composés, affichés et exportés dans `<sauvegarde>.visuals`. `-visual-dir` change ce dossier. En mode terminal, `-visuals` active l'export et affiche les chemins des PNG.

Seuls les calques approuvés par `Engine.View().Visuals` sont composés. Une image affichée est un résultat décoratif : son contenu ne modifie aucune règle. Un chemin mentionné par le joueur ou le modèle reste du texte. Sans visuels de catalogue, un encart indique leur absence ; `-art` peut afficher les dessins ASCII validés du LLM en remplacement, avec leurs limites de 80 × 24. Le guide [catalogue.md](catalogue.md) explique l'import des fonds et sprites.

## Application macOS

```sh
make app                   # build/AI Venture.app, avec runtime llama-server
make app WITH_LLAMA=0      # fenêtre et jeu configuré sans runtime natif
make app GAME=autre-jeu/game.json # contenu interchangeable
```

Double-cliquer sur **AI Venture.app** lance le contenu choisi par `Contents/Resources/game.json`, en anglais et hors ligne. Le binaire ne contient aucun scénario. Le catalogue `catalogue-<id>.sqlite`, les sauvegardes `savegame-<id>-<lang>.json` et les exports restent dans `~/Library/Application Support/ai-venture`. `-data-dir` change cet emplacement. Un autre identifiant de jeu conserve les données des aventures précédentes séparées. Voir [game-config.md](game-config.md) pour remplacer le contenu sans changer Go.

Pour choisir le français ou un LLM, lancer l'exécutable de l'application avec les options habituelles :

```sh
"build/AI Venture.app/Contents/MacOS/aventure" -offline -lang fr
"build/AI Venture.app/Contents/MacOS/aventure" -lang fr -provider llamacpp \
  -gguf "$PWD/models/llama-3.2-3b-instruct-q4_k_m.gguf"
```

Fournir des chemins absolus pour les fichiers externes passés au `.app` (`-game`, `-gguf`, `-scenario`, `-catalogue`, `-save`, etc.) : sur macOS, Ebitengine place son répertoire de travail dans `Contents/Resources` avant le démarrage. Le binaire ordinaire `bin/aventure` conserve le répertoire courant habituel.

Les archives macOS créées par `make dist` incluent cette application. Elle contient les exemples, ses licences et son manifeste ; les modèles et données personnelles restent exclus. Ce prototype ne possède pas encore de signature Developer ID, de notarisation ni d'installateur.

## Plateformes et compilation

Go 1.26 et les dépendances épinglées dans `go.mod` suffisent pour compiler le jeu sans CGO, y compris son interface Ebitengine 2.10. Le runtime llama-server reste une compilation native séparée. Sous Linux, la fenêtre nécessite un environnement de bureau X11 ou compatible XWayland et un pilote graphique pris en charge par Ebitengine ; pour une machine sans écran, utiliser `-ui terminal`. Sous Windows, le binaire distribué utilise le sous-système graphique pour éviter une console supplémentaire. Les tests Go n'ouvrent aucune fenêtre et n'exigent pas de LLM.
