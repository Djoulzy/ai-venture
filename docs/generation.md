# Générer une aventure depuis un pitch

L'agent de création consulte le **catalogue permanent SQLite**, sélectionne des fiches dans la langue demandée et propose une courte quête. Go construit les règles, vérifie le scénario et rejoue un parcours gagnant avant de publier le JSON. Le cache RAG de la narration, les sauvegardes et l'historique du joueur ne servent pas à la création.

Cette commande sert à la **création autonome par le jeu**. Codex peut aussi écrire directement une aventure dans le workspace à partir des mêmes fiches, sans lancer de modèle local, puis la valider avec Go. [Providence — La dette de la marée](providence-tides.md) est un exemple de ce second mode, fourni en français et en anglais.

## Créer puis jouer

Depuis la racine du workspace, avec le modèle déjà disponible :

```sh
make build
mkdir -p build/providence
./bin/catalogue import -source data/catalogue/providence -db build/providence/catalogue.sqlite

./bin/aventure -generate -lang fr \
  -catalogue build/providence/catalogue.sqlite \
  -pitch "Providence, 1921. Retrouver le Necronomicon dans la bibliothèque, déchiffrer son rituel puis apaiser la créature du passage souterrain." \
  -scenario-out build/providence/mon-aventure-fr.json \
  -provider llamacpp -gguf ./models/llama-3.2-3b-instruct-q4_k_m.gguf \
  -timeout 120s

./bin/aventure -lang fr \
  -scenario build/providence/mon-aventure-fr.json \
  -catalogue build/providence/catalogue.sqlite \
  -save build/providence/mon-aventure-fr.save.json \
  -provider llamacpp -gguf ./models/llama-3.2-3b-instruct-q4_k_m.gguf
```

La première commande de jeu génère le fichier puis quitte, sans ouvrir de fenêtre. La seconde ouvre la partie illustrée. Pour jouer sans modèle, remplacer les options `-provider ... -gguf ...` de la seconde commande par `-offline`. Ajouter `-load` pour reprendre la sauvegarde ; cela ne régénère jamais le scénario.

Avec Ollama, remplacer les options du fournisseur par `-provider ollama -model lfm3:1.7b`. Pour l'anglais, utiliser `-lang en`, un pitch anglais et des noms de fichiers distincts. Les textes du catalogue ne sont pas traduits automatiquement. Dans une distribution, utiliser `./aventure` et `./catalogue` au lieu de `./bin/...` ; Go n'est pas nécessaire.

Le dossier de destination doit exister. Le fichier de scénario doit être **nouveau** : aucune génération n'écrase un fichier existant. Conserver ce JSON avec sa sauvegarde. Il contient les textes sélectionnés, les règles et la provenance ; une modification ultérieure du catalogue ne change pas ces textes. Les images restent référencées par leur identifiant dans le catalogue SQLite, à conserver pour les visuels.

## Options et sélection des fiches

| Option | Effet |
| --- | --- |
| `-generate` | Créer un scénario puis quitter |
| `-pitch` | Intention narrative, au plus 2 000 caractères Unicode et 4 096 octets |
| `-catalogue` | Base permanente importée ; au moins un lieu et un objet dans la langue choisie |
| `-scenario-out` | Nouveau fichier JSON ; chemin choisi par l'utilisateur |
| `-lang en\|fr` | Langue des fiches recherchées, des prompts et des messages |
| `-seed` | Identifiant de variation inscrit dans la provenance ; `0` choisit une valeur aléatoire |
| `-timeout` | Délai par appel au modèle ; trois tentatives maximum |

La graine fait partie du contexte de création et de l'identifiant du scénario. Elle ne garantit pas une reproduction exacte des sorties du LLM : conserver le fichier généré pour rejouer la même aventure.

La recherche FTS5 classe les fiches selon les termes du pitch, séparément par langue et type. Elle retient au maximum quatre lieux, quatre objets, deux monstres, un personnage et une situation. Si les correspondances lexicales sont insuffisantes, elle complète avec des fiches du même type et de la même langue, dans un ordre stable. Il ne s'agit pas d'une recherche par embeddings. Utiliser dans le pitch les noms et thèmes présents dans les fiches aide la sélection.

Le modèle reçoit seulement identifiants, types, titres et extraits de 600 caractères par fiche. Il ne reçoit ni pixels, ni chemins, ni métadonnées techniques. Les descriptions complètes sont reprises par Go dans le scénario. Les fiches gagnent donc à commencer par leurs caractéristiques utiles à une intrigue.

`-llama-context` passe par défaut à **8192** en mode génération, pour le serveur démarré par le programme. Une valeur explicite reste prioritaire. Un serveur réutilisé conserve son propre réglage. Les autres options de fournisseur et de supervision restent identiques au jeu. `-offline`, `-load` et `-scenario` sont incompatibles avec `-generate`.

## Proposition et validation

Le plan JSON du modèle contient exactement :

```json
{
  "title": "Le rituel des profondeurs",
  "introduction": "Retrouvez le livre et étudiez-le avant de présenter son rituel à la créature.",
  "locations": ["providence_library", "dark_passage"],
  "entities": [
    {"entry": "book_necronomicon", "location": "providence_library"},
    {"entry": "monster_profond", "location": "dark_passage"}
  ],
  "steps": [
    {"tool": "book_necronomicon", "target": "", "location": "providence_library", "message": "Vous déchiffrez le rituel."},
    {"tool": "book_necronomicon", "target": "monster_profond", "location": "dark_passage", "message": "La créature s'apaise à la lecture du rituel."}
  ],
  "situation": ""
}
```

Ce plan interne n'est pas le fichier final de scénario. Les identifiants autorisés dépendent des résultats de la recherche. Go refuse les champs inconnus, manquants, doublons ou nuls et revalide toutes les références, même si le fournisseur ignore le schéma JSON.

Go relie les lieux dans les deux sens, place les entités et construit des règles d'utilisation ordonnées. Chaque étape exige la réussite de la précédente et ne peut réussir qu'une fois. L'objectif correspond à la dernière étape. Après `world.Validate`, les fiches sont matérialisées en documents à découverte locale. Un moteur temporaire exécute un parcours qui collecte les outils et accomplit toutes les étapes ; un échec interdit la publication. La solution n'est ni enregistrée dans le JSON, ni donnée au narrateur. La partie démarre avec un moteur neuf.

Les refus de validation sont renvoyés au modèle pour correction, dans une limite de trois propositions. Si les lieux et placements sont déjà valides, le schéma de correction les conserve et contraint les interactions à la position de chaque cible fixe. Go vérifie encore toute la réponse. Une erreur HTTP ou un délai dépassé arrête la génération. Aucun fichier final n'est créé après un échec ; une publication valide est atomique et sans remplacement.

## Périmètre de cette version

- Une courte quête : **1 à 4 lieux**, au plus **12 entités**, **4 entités par lieu** et **1 à 6 interactions ordonnées**. Les limites du catalogue sélectionné peuvent réduire ces nombres.
- Les objets sont transportables. Les personnages et monstres sont des cibles fixes que l'on peut observer ou sur lesquelles utiliser un objet. Il n'y a pas encore de combats chiffrés, dialogues libres, consommation ou disparition d'entités.
- Tous les lieux restent accessibles, reliés par `avant` / `arriere` en français et `forward` / `back` en anglais. Le générateur ne crée pas encore de carte à embranchements ou de verrous de déplacement.
- Go utilise la première image de chaque fiche retenue : fond pour un lieu, sprite proportionné pour une entité. Les sprites disparaissent du décor lorsqu'un objet est pris. La composition habituelle produit les PNG ; aucune nouvelle image n'est générée.
- La vérification prouve qu'un parcours gagnant existe avec les règles du moteur. La qualité littéraire, la clarté des indices et la fidélité au pitch restent dépendantes du modèle. Lire l'introduction du scénario permet d'évaluer ces aspects avant de le partager.

L'agent fonctionne pour l'instant en ligne de commande avant la partie. `generation.Source` implémente `world.Source` pour permettre ensuite un écran de création au premier lancement, sans changer le moteur ni la persistance.
