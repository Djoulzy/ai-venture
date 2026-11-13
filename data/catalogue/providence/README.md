# Providence — Le seuil / The Threshold

Le catalogue fournit quatre lieux (demeure, rue, bibliothèque et passage souterrain), le Necronomicon et un monstre, dans les deux langues, avec six images. Deux scénarios fixes permettent de jouer devant la demeure :

- `scenarios/providence-fr.json` : français, identifiant `providence_le_seuil_fr`.
- `scenarios/providence-en.json` : anglais, identifiant `providence_the_threshold_en`.

Ils possèdent **un seul lieu**, devant la maison, et un objectif atteignable : prendre la clé ancienne puis ouvrir la porte. La clé et la porte sont des objets définis dans les scénarios ; elles n'ont pas encore de sprites séparés. Les autres fiches sont disponibles pour l'agent de création ; elles ne sont pas ajoutées automatiquement à ces scénarios fixes. Aucun LLM n'est nécessaire pour cette première scène.

## Préparer la partie

Depuis la racine du projet :

```sh
make build
mkdir -p build/providence
./bin/catalogue import -source data/catalogue/providence -db build/providence/catalogue.sqlite
```

L'import conserve les fiches et images référencées dans SQLite. Il est possible de le répéter après modification des sources ; il n'utilise pas `-replace`. Les images non référencées dans les manifestes restent des sources de travail et ne sont pas copiées dans les distributions. Une catégorie vide en préparation, telle que `personnages/personnages.json`, est acceptée avec les autres catégories.

Pour créer une aventure originale à partir des lieux, du livre et du monstre, suivre le [guide de génération depuis un pitch](../../../docs/generation.md). La commande `-generate` écrit un nouveau scénario après validation des règles et d'un parcours gagnant ; la partie l'utilise ensuite avec `-scenario`.

Une aventure écrite directement par Codex utilise aussi les quatre lieux, le livre et le monstre : [La dette de la marée](../../../docs/providence-tides.md), dans `scenarios/providence-tides-fr.json` et `scenarios/providence-tides-en.json`. Sa création et son mode de jeu hors ligne ne nécessitent ni Ollama ni llama.cpp.

## Jouer en français, sans modèle

```sh
./bin/aventure -offline -lang fr \
  -scenario scenarios/providence-fr.json \
  -catalogue build/providence/catalogue.sqlite \
  -save build/providence/savegame-fr.json
```

La fenêtre affiche automatiquement la façade. Les sauvegardes vont dans `build/providence/savegame-fr.json` et les scènes PNG dans `build/providence/savegame-fr.json.visuals/`. Le dossier `build/` est ignoré par Git ; conserver une copie des sauvegardes à préserver.

Quelques commandes locales pour terminer la scène :

```text
/regarder
/observer cle_ancienne
/prendre cle_ancienne
/inventaire
/utiliser cle_ancienne sur porte
/sauver
```

La clé doit être portée avant de pouvoir être utilisée. `/deposer cle_ancienne` permet de tester le dépôt. La porte est fixe et ne peut pas être prise. Il n'existe pas encore de sortie vers l'intérieur : ouvrir la porte accomplit l'objectif sans créer un nouveau lieu.

Pour reprendre la sauvegarde, relancer la même commande avec `-load`. Pour jouer dans un terminal tout en exportant les PNG, ajouter `-ui terminal -visuals`.

## Lancer par double-clic sur macOS

```sh
make app
```

Le contenu du bundle dépend de `game.json`. La configuration du workspace sélectionne **La dette de la marée**, en anglais et hors ligne au double-clic. Pour choisir le français :

```sh
"build/AI Venture.app/Contents/MacOS/aventure" -offline -lang fr
```

L'application utilise l'identifiant de `game.json` pour nommer sa base et ses sauvegardes dans `~/Library/Application Support/ai-venture`. Les anciennes données de démonstration restent séparées. Les archives `make dist GAME=...` incluent les scénarios et les manifestes sélectionnés par cette configuration, ainsi que leurs images référencées, sans base SQLite personnelle.

## Jouer avec Llama 3.2

```sh
./bin/aventure -lang fr -provider llamacpp \
  -gguf ./models/llama-3.2-3b-instruct-q4_k_m.gguf \
  -scenario scenarios/providence-fr.json \
  -catalogue build/providence/catalogue.sqlite \
  -save build/providence/savegame-fr.json
```

Les commandes peuvent alors être formulées librement, par exemple « Je ramasse la clé ancienne » puis « Je déverrouille la porte avec la clé ». Le préfixe `/` conserve les commandes locales. Le narrateur peut consulter la fiche de la demeure via le RAG ; les règles et les changements d'état restent validés par Go.

Avec Ollama, remplacer `-provider llamacpp -gguf ...` par `-provider ollama -model lfm3:1.7b`. Le modèle doit déjà être disponible localement.

## Jouer en anglais

```sh
./bin/aventure -offline -lang en \
  -scenario scenarios/providence-en.json \
  -catalogue build/providence/catalogue.sqlite \
  -save build/providence/savegame-en.json
```

Utiliser `/examine old_key`, `/take old_key`, `/use old_key on door`, puis `/save`. La sauvegarde anglaise est `build/providence/savegame-en.json`. Les deux variantes ont des identifiants et des empreintes distincts ; elles ne partagent pas leur sauvegarde.

Pour étendre la bibliothèque, voir [le guide du catalogue](../../../docs/catalogue.md). Garder les scénarios dans `scenarios/` : tous les fichiers JSON du dossier donné à `catalogue import -source` sont interprétés comme des manifestes de catalogue.

Dans une archive distribuée, utiliser `./aventure` et `./catalogue` à la place des chemins `./bin/...` ; les étapes de compilation Go ne sont pas nécessaires.
