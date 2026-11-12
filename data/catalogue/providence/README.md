# Providence — Le seuil / The Threshold

Le catalogue `lieux/lieux.json` fournit la demeure coloniale en français et en anglais, avec son image de façade. Deux scénarios fixes permettent de jouer avec ce contenu :

- `scenarios/providence-fr.json` : français, identifiant `providence_le_seuil_fr`.
- `scenarios/providence-en.json` : anglais, identifiant `providence_the_threshold_en`.

Ils possèdent **un seul lieu**, devant la maison, et un objectif atteignable : prendre la clé ancienne puis ouvrir la porte. La clé et la porte sont des objets définis dans les scénarios ; elles n'ont pas encore de sprites séparés. L'intérieur de la demeure, les autres lieux, personnages et combats restent à créer. Aucun LLM n'est nécessaire pour cette première scène.

## Préparer la partie

Depuis la racine du projet :

```sh
make build
mkdir -p build/providence
./bin/catalogue import -source data/catalogue/providence -db build/providence/catalogue.sqlite
```

L'import conserve vos fiches et votre image dans SQLite. Il est possible de le répéter après modification des sources ; il n'utilise pas `-replace`. Dans le workspace, quatre autres vues présentes dans `lieux/images/` restent disponibles comme sources, mais ne sont pas encore référencées par le manifeste ni utilisées pendant la partie. Elles ne sont donc pas copiées dans les distributions. Leurs pixels et le manifeste existant ne sont pas modifiés.

## Jouer en français, sans modèle

```sh
./bin/aventure -offline -lang fr \
  -scenario scenarios/providence-fr.json \
  -catalogue build/providence/catalogue.sqlite \
  -data-dir build/providence
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

Ouvrir `build/AI Venture.app` lance désormais Providence en anglais, hors ligne. Pour choisir le français :

```sh
"build/AI Venture.app/Contents/MacOS/aventure" -offline -lang fr
```

L'application importe son propre `catalogue-providence.sqlite` et utilise `savegame-providence-fr.json` ou `savegame-providence-en.json` dans `~/Library/Application Support/ai-venture`. Les anciennes données de démonstration restent séparées. Les archives `make dist` incluent les deux scénarios et uniquement la façade référencée par le catalogue, sans base SQLite personnelle.

## Jouer avec Llama 3.2

```sh
./bin/aventure -lang fr -provider llamacpp \
  -gguf ./models/llama-3.2-3b-instruct-q4_k_m.gguf \
  -scenario scenarios/providence-fr.json \
  -catalogue build/providence/catalogue.sqlite \
  -data-dir build/providence
```

Les commandes peuvent alors être formulées librement, par exemple « Je ramasse la clé ancienne » puis « Je déverrouille la porte avec la clé ». Le préfixe `/` conserve les commandes locales. Le narrateur peut consulter la fiche de la demeure via le RAG ; les règles et les changements d'état restent validés par Go.

Avec Ollama, remplacer `-provider llamacpp -gguf ...` par `-provider ollama -model lfm3:1.7b`. Le modèle doit déjà être disponible localement.

## Jouer en anglais

```sh
./bin/aventure -offline -lang en \
  -scenario scenarios/providence-en.json \
  -catalogue build/providence/catalogue.sqlite \
  -data-dir build/providence
```

Utiliser `/examine old_key`, `/take old_key`, `/use old_key on door`, puis `/save`. La sauvegarde anglaise est `build/providence/savegame-en.json`. Les deux variantes ont des identifiants et des empreintes distincts ; elles ne partagent pas leur sauvegarde.

Pour étendre la bibliothèque, voir [le guide du catalogue](../../../docs/catalogue.md). Garder les scénarios dans `scenarios/` : tous les fichiers JSON du dossier donné à `catalogue import -source` sont interprétés comme des manifestes de catalogue.

Dans une archive distribuée, utiliser `./aventure` et `./catalogue` à la place des chemins `./bin/...` ; les étapes de compilation Go ne sont pas nécessaires.
