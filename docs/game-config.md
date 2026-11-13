# Jeux interchangeables

Le binaire ne contient aucun scénario jouable. Il charge des fichiers externes ; remplacer l'aventure ne nécessite aucune modification ni recompilation du moteur.

## Choisir le contenu

```sh
./bin/aventure -offline -lang fr -game /chemin/mon-jeu/game.json
```

La configuration sélectionne les fichiers et le catalogue :

```json
{
  "version": 1,
  "id": "mon_jeu",
  "scenarios": {
    "en": "scenarios/adventure-en.json",
    "fr": "scenarios/adventure-fr.json"
  },
  "catalogue_source": "catalogue",
  "documents": ["guide.md"]
}
```

Les chemins sont relatifs au dossier de cette configuration, quel que soit le répertoire depuis lequel le jeu est lancé. Ils restent dans cette racine ; les chemins absolus, sorties par `..` et liens symboliques vers l'extérieur sont refusés. Le JSON est strict, sans doublons ni champs inconnus.

`id` contient de 1 à 64 lettres minuscules, chiffres ou `_`, et commence par une lettre. `scenarios` contient une ou deux variantes `fr` / `en`. Une langue absente provoque une erreur explicite ; aucun autre scénario n'est choisi silencieusement. `catalogue_source` est facultatif : dossier de manifestes ou manifeste unique, dans le format de [catalogue.md](catalogue.md). `documents` est une liste facultative de guides `.md`, `.txt` ou `.pdf` à inclure dans la distribution.

Le catalogue est importé au premier lancement dans `catalogue-<id>.sqlite`, dans `-data-dir` ou le dossier courant. Une base existante est conservée. Pour actualiser volontairement son contenu, utiliser `catalogue import` ; pour distribuer une aventure distincte, choisir un nouvel identifiant. Les sauvegardes par défaut sont `savegame-<id>-<lang>.json`. `-catalogue` et `-save` permettent de fournir explicitement une base existante et un chemin de sauvegarde.

Pour charger directement un seul scénario, sans configuration :

```sh
./bin/aventure -offline -lang fr \
  -scenario /chemin/aventure.json \
  -catalogue /chemin/catalogue.sqlite \
  -save /chemin/ma-partie.json
```

`-game` et `-scenario` sont exclusifs. Avec `-scenario`, aucune configuration n'est chargée automatiquement. Sans `-save`, le nom du fichier utilise l'identifiant du scénario et la langue. Un scénario sans documents de catalogue ni visuels peut se jouer sans `-catalogue`.

Sans ces options, un exécutable ordinaire cherche `game.json` dans le dossier courant, puis à côté du binaire. L'application macOS utilise `Contents/Resources/game.json` et conserve ses données dans `~/Library/Application Support/ai-venture`, sauf `-data-dir` explicite. Une configuration explicite `-game` reste prioritaire. Sans configuration ni scénario, le programme indique comment en sélectionner un ; il ne possède aucun scénario de repli.

## Distribuer un autre jeu

```sh
make app GAME=/chemin/mon-jeu/game.json
make dist GAME=/chemin/mon-jeu/game.json VERSION=0.2.0
```

Le défaut de `GAME` est le fichier `game.json` du workspace. Son choix actuel est une donnée éditable. Les scripts n'ont connaissance d'aucun titre d'aventure, identifiant de lieu ou nom de catalogue particulier.

Le paquet reçoit cette configuration sous le nom `game.json`, les variantes sélectionnées, les manifestes de la source du catalogue, leurs images référencées et les guides déclarés. Les bases personnelles, sauvegardes, modèles et images non référencées sont exclus. Les licences, manifestes de build et sommes SHA-256 restent fournis. Les fichiers de contenu peuvent être remplacés ensuite, en conservant leurs chemins relatifs ou en modifiant la configuration.

Les fixtures historiques sont confinées dans `internal/testfixtures/testdata` et importées uniquement par les tests. Elles ne sont pas liées à l'exécutable ni incluses dans les paquets. Un scénario fixe écrit à la main ou par Codex et un scénario issu de `-generate` suivent tous la même validation `world.Validate`.
