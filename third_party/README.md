# Dépendance native llama.cpp

Les sources officielles sont présentes localement dans `llama.cpp/`, un clone Git indépendant ignoré par le dépôt du jeu. Le script [build-llama-server.sh](../scripts/build-llama-server.sh) les récupère et vérifie leur révision exacte avant compilation.

- Dépôt : https://github.com/ggml-org/llama.cpp
- Version : **v0.8.0**, également identifiée par **b11960**.
- Commit : `8642541b255bfb5dc39cd351a178be09013a55d6`.
- Licence : MIT, conservée dans les sources et copiée dans `runtime/llama.cpp.LICENSE`.
- Compatibilité : cette version introduit le support de LFM3-1.7B ; voir les [notes de version officielles](https://github.com/ggml-org/llama.cpp/releases/tag/v0.8.0).

La révision est fixée dans le script. Le script refuse un clone à une autre révision ou comportant des modifications suivies par Git ; il ne réinitialise jamais les sources existantes. Pour changer de version, mettre à jour le tag et le SHA attendus dans le script, puis préparer explicitement le clone correspondant.

Le clone n'est pas un sous-module du dépôt principal. Sur un nouveau checkout du jeu, exécuter le script de compilation pour le recréer. Les sources et les fichiers générés ne sont pas ajoutés aux commits du jeu.
