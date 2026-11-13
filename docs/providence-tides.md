# Providence — La dette de la marée

Cette aventure a été écrite directement par Codex à partir du catalogue Providence, sans appel à Ollama ni à llama.cpp. Elle existe en français et en anglais. Les fichiers ont passé la validation commune du moteur et un parcours gagnant, avec vérification des refus sans mutation et de la sauvegarde.

## Pitch

Providence, novembre 1921. Trois coups montent des pavés d'une rue loin du rivage. Une demeure abandonnée appelle quelque chose dans les profondeurs, et une voix commence à prononcer votre nom. Le Necronomicon pourrait expliquer ce phénomène. Encore faut-il distinguer un pacte d'une invocation, et un témoin d'un ennemi.

L'aventure relie les quatre lieux du catalogue : ruelle, bibliothèque, demeure coloniale et souterrain. Elle utilise le Necronomicon et le Profond, ainsi que leurs images existantes. Les textes d'origine du catalogue sont conservés ; des indices et des passages narratifs propres à cette aventure les complètent. Aucun combat, personnage mobile ou compte à rebours n'est ajouté.

## Jouer dans la fenêtre

Depuis la racine du workspace, avec les exécutables déjà compilés :

```sh
mkdir -p build/providence
./bin/catalogue import -source data/catalogue/providence -db build/providence/catalogue.sqlite

./bin/aventure -offline -lang fr \
  -scenario scenarios/providence-tides-fr.json \
  -catalogue build/providence/catalogue.sqlite \
  -save build/providence/tides-fr.save.json
```

Le mode `-offline` permet de jouer sans aucun modèle local. Les déplacements utilisent les directions affichées. On peut examiner les objets, les prendre, les déposer et les utiliser ; par exemple `/prendre book_necronomicon` puis `/lire le Necronomicon` à la bibliothèque. Devant la demeure, `/lire le Necronomicon` permet ensuite d’interpréter le signe. Les variantes `étudier` et `read` / `study` sont également reconnues selon la langue. Ces commandes se saisissent directement dans le champ de la fenêtre ; les boutons restent des actions génériques. Les descriptions et les résultats des interactions suffisent à guider la progression.

Pour l'anglais, choisir `-lang en`, `scenarios/providence-tides-en.json` et une sauvegarde distincte `build/providence/tides-en.save.json`. Ajouter `-load` à la commande choisie pour reprendre la partie. Ajouter `-ui terminal -visuals` pour jouer dans un terminal avec export des PNG.

Si l'on souhaite des commandes libres et une narration supplémentaire pendant la partie, remplacer `-offline` par les options habituelles d'Ollama ou de llama.cpp. Cela ne recrée pas le scénario : son intrigue et ses règles restent celles des fichiers fournis.

## Création accompagnée et création autonome

Pour une nouvelle aventure accompagnée, donner ici un pitch, le catalogue à employer, une langue et éventuellement une durée ou un ton. Codex peut consulter les fiches, écrire l'intrigue et les règles, puis les valider avec Go. Il n'est pas nécessaire de lancer un autre LLM pour cette écriture.

La fonction [`-generate`](generation.md) conserve son utilité : elle permet au jeu de créer une aventure localement à partir d'un pitch, sans session Codex. Ollama et llama.cpp sont les deux fournisseurs disponibles pour cette fonction autonome. Le fichier final, dans les deux modes, est un scénario JSON validé et chargé par le même moteur.

Les nouveaux scénarios portent la provenance `codex-authored-v1`. Ils incluent une copie des textes documentaires sélectionnés : un réimport ultérieur ne réécrit pas leur intrigue. Les images restent dans le catalogue SQLite. Conserver chaque fichier de scénario avec ses sauvegardes ; les variantes française et anglaise ont des identifiants distincts.
