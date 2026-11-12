# Catalogue permanent et visuels PNG

Le catalogue conserve des fiches de lieux, personnages, monstres, objets et situations dans une base SQLite indépendante du cache RAG. Les images sont importées dans cette même base sous forme de PNG normalisés. Aucun serveur ni modèle n'est nécessaire pour importer, rechercher ou composer les images.

Cette étape fournit la bibliothèque de contenu et ses références dans un scénario écrit à la main. La génération automatique d'un scénario, les dialogues de PNJ et les combats restent à ajouter. Une fiche de personnage ou de monstre ne crée pas automatiquement une entité interactive.

## Essayer l'exemple

```sh
make build
./bin/catalogue import -source data/catalogue/providence -db catalogue.sqlite
./bin/catalogue list -db catalogue.sqlite -lang fr
./bin/catalogue search -db catalogue.sqlite -lang fr -query Providence
./bin/catalogue assets -db catalogue.sqlite

./bin/aventure -offline -lang fr \
  -scenario scenarios/providence-fr.json -catalogue catalogue.sqlite \
  -visuals -visual-dir ./savegame-providence.visuals -save savegame-providence-fr.json
```

En anglais, utiliser `-lang en -scenario scenarios/providence-en.json -save savegame-providence-en.json`. La fenêtre graphique affiche directement les scènes ; leur composition est automatique pour un scénario lié au catalogue. Les PNG sont aussi exportés au démarrage, au chargement et lorsque le jeu réaffiche un lieu après une action. Avec `-ui terminal -visuals`, seul leur chemin apparaît dans le terminal. Voir [window.md](window.md).

Providence contient un lieu décrit dans les deux langues et une image de façade référencée par le catalogue. La première scène consiste à prendre une clé puis à ouvrir la porte, sans accéder encore à l'intérieur. Voir le [guide de lancement Providence](../data/catalogue/providence/README.md). Le catalogue géométrique `data/catalogue/demo` reste un exemple d'import de sprites ; ses anciens scénarios `catalogue-fr.json` et `catalogue-en.json` ont été retirés.

Pour activer la narration avec les fiches découvertes :

```sh
./bin/aventure -lang fr -provider llamacpp \
  -gguf ./models/llama-3.2-3b-instruct-q4_k_m.gguf \
  -scenario scenarios/providence-fr.json -catalogue catalogue.sqlite \
  -save savegame-providence-fr.json -visuals
```

Dans une archive distribuée, utiliser `./catalogue` et `./aventure` à la place des chemins `./bin/...`. Go n'est pas requis. Les exemples et les scénarios sont inclus ; aucune base personnelle n'y est ajoutée.

## Écrire les fiches et importer les images

Un fichier JSON décrit un lot. Un dossier peut contenir plusieurs lots, éventuellement dans des sous-dossiers ; l'import traite l'ensemble dans une seule transaction.

```json
{
  "version": 1,
  "entries": [
    {
      "id": "port_ancien",
      "kind": "location",
      "language": "fr",
      "title": "Le vieux port",
      "text": "Le vieux port abrite une communauté de pêcheurs. Les maisons sont bâties en pierre claire.",
      "tags": ["port", "cote", "1987"],
      "metadata": {"climat": "oceanique", "ambiance": "isolee"},
      "images": ["port_fond"]
    }
  ],
  "assets": [
    {
      "id": "port_fond",
      "kind": "location",
      "path": "images/port.png",
      "license": "Votre mention de licence ou de provenance"
    }
  ]
}
```

`kind` accepte `location`, `character`, `monster`, `item`, `situation` pour les fiches. Les images utilisent les quatre premiers types. Les métadonnées sont des paires clé/valeur textuelles ; elles ne sont pas exécutées comme des règles. Les noms des champs restent en anglais, quelle que soit la langue du texte.

Les chemins des images sont relatifs au JSON. Pour un dossier importé, ils peuvent référencer ses autres sous-dossiers mais jamais sortir de la racine, y compris via un lien symbolique. Une URL ne déclenche aucun téléchargement. Formats : PNG, JPEG, GIF (première image uniquement). Les sprites PNG transparents permettent les superpositions. Un JPEG est opaque ; le compositeur ne détoure pas son sujet.

```sh
./bin/catalogue import -db catalogue.sqlite -source ./mon-catalogue
./bin/catalogue show -db catalogue.sqlite -id port_ancien -lang fr
./bin/catalogue search -db catalogue.sqlite -lang fr -kind location -query port -limit 20
```

L'import insère ou met à jour les fiches par couple `(id, language)` et les images par `id`, en conservant les autres données. Un réimport ne crée pas de doublons. Les images référencées doivent exister dans le catalogue ou le même lot. `-replace` remplace explicitement **tout** le catalogue ; une erreur annule également ce remplacement. Les sorties des commandes sont du JSON. `catalogue help` affiche les options.

Les sources peuvent ensuite être déplacées : le jeu lit les images copiées dans SQLite. Modifier une image sur disque ne change pas le catalogue avant réimport. La recherche utilise FTS5 avec des termes littéraux, les accents ignorés et un filtre de langue/type ; elle n'utilise pas d'embeddings et ne traduit pas les requêtes.

Limites par import : 128 manifestes, 2 048 fiches, 128 images et 128 Mio d'images normalisées. Une fiche possède un titre de 120 caractères, un texte de 2 000 caractères, au plus 16 tags, 32 métadonnées et 8 références d'images. Chaque image est limitée à 16 Mio, 4 096 pixels par côté et 8 mégapixels au total. Chaque JSON est limité à 1 Mio. Les identifiants utilisent une lettre minuscule suivie de lettres minuscules, chiffres ou `_`, sur 64 caractères maximum.

## Préparer les visuels

Le moteur compose les scènes en **640 × 384 pixels**, soit un **ratio de 5:3**. Pour générer ou dessiner les images sources, utiliser les résolutions suivantes :

| Visuel | Résolution conseillée | Format |
| --- | --- | --- |
| Décor de lieu | **640 × 384 px** | PNG opaque |
| Personnage ou monstre | 128 × 192 px | PNG avec fond transparent |
| Objet | 64 × 64 ou 128 × 128 px | PNG avec fond transparent |

Les tailles des personnages, monstres et objets sont des recommandations, pas des contraintes d'import. Le scénario fixe leur position et leur taille d'affichage dans les calques `visuals`. Conserver les proportions de chaque image dans son rectangle de destination évite les déformations ; voir [Composer les scènes avec Go](#composer-les-scènes-avec-go).

Garder un style graphique, une perspective et un éclairage cohérents entre les décors et les sprites pour faciliter leur assemblage. Préparer les personnages et objets sur un véritable fond transparent : le compositeur ne retire pas automatiquement un fond opaque.

## Rattacher une fiche au RAG

Ajouter au scénario :

```json
"catalogue_documents": [
  {"id": "histoire_port", "entry": "port_ancien", "location": "quai"}
]
```

`location` est un identifiant déjà défini dans `locations`. Comme les documents ordinaires, une référence peut combiner `location`, `item` et `conditions`. Sans condition, la fiche est connue dès le départ. Le moteur sélectionne la langue de la partie, copie le titre et le texte dans les documents du scénario, puis applique les conditions de découverte. Il ne transmet pas au LLM les métadonnées, les références d'images ou le reste du catalogue.

L'empreinte du scénario résolu inclut ces textes. Une mise à jour d'une fiche utilisée peut rendre une ancienne sauvegarde incompatible ; conserver une copie/version du catalogue pour les aventures commencées. Les images sont décoratives et leurs pixels ne changent pas les règles ou l'état sauvegardé.

## Composer les scènes avec Go

Le scénario définit des calques dans `visuals` :

```json
"visuals": [
  {
    "asset": "port_fond", "location": "quai",
    "x": 0, "y": 0, "width": 100, "height": 100, "z": 0
  },
  {
    "asset": "gardien_sprite", "location": "quai",
    "x": 25, "y": 35, "width": 12, "height": 55, "z": 10
  },
  {
    "asset": "cle_sprite", "location": "quai", "item": "cle",
    "x": 45, "y": 80, "width": 10, "height": 8, "z": 20
  }
]
```

Toutes les images mentionnées doivent avoir été importées. Le canevas fait **640 × 384 pixels**. Coordonnées et tailles sont des pourcentages entiers ; chaque rectangle doit rester dans le canevas. Les images sont redimensionnées au rectangle demandé, sans conservation automatique des proportions. Préparer des images avec le même rapport largeur/hauteur que leur rectangle évite les déformations. Le redimensionnement au plus proche voisin convient au pixel art.

Les calques sont dessinés par `z` croissant. À `z` égal, l'ordre du tableau est conservé. Go applique la transparence alpha. Limites : 32 calques par lieu, 512 par scénario. `item` exige la présence de l'objet dans le lieu courant : la clé disparaît de l'image quand elle rejoint l'inventaire. Les `conditions` sur les drapeaux sont aussi possibles.

Le moteur ne donne au compositeur que les calques actuellement visibles ; les identifiants d'images sont exclus du JSON d'inférence. Le compositeur ne déduit pas les placements depuis le texte et ne fait pas de mélange génératif : il superpose et redimensionne vos images selon le scénario. Personnages et monstres peuvent être illustrés dès maintenant sans supposer un système de PNJ ou de combats.

L'export est activé par `-visuals` et son dossier par `-visual-dir` (défaut : `<sauvegarde>.visuals`). Le nom dépend des calques, placements et empreintes des images. Une même scène utilise le même nom ; un changement d'objet ou d'image produit un autre fichier. Chaque export est écrit atomiquement et reconstruit depuis le catalogue. Un PNG exporté modifié ne devient jamais une source de données. Les anciens PNG restent dans le dossier jusqu'à leur suppression manuelle. Ce dossier est un export, pas une sauvegarde.

L'export fonctionne hors ligne, avec les commandes `/`, avec ou sans narration et indépendamment de `-art`, qui conserve l'illustrateur LLM ASCII/ANSI limité à 80 × 24. En cas d'échec d'export, la partie continue avec les faits du moteur. Aucune image n'est envoyée au LLM, au RAG, à un service distant ou à un logiciel externe.
